package oauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/store"
	"github.com/mananuf/justbarme/internal/store/sqlc"
)

const providerGoogle = "google"

// IDTokenVerifier verifies a raw Google-issued ID token and extracts its
// claims. Kept as an interface -- same reasoning as internal/email.Provider
// -- so tests never need real network access or a real Google OAuth client.
type IDTokenVerifier interface {
	Verify(ctx context.Context, rawIDToken string) (GoogleClaims, error)
}

type Service struct {
	pool     *pgxpool.Pool
	identity *identity.Service
	verifier IDTokenVerifier
}

func New(pool *pgxpool.Pool, identitySvc *identity.Service, verifier IDTokenVerifier) *Service {
	return &Service{pool: pool, identity: identitySvc, verifier: verifier}
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("generate id: %w", err)
	}
	return id, nil
}

// SignInWithGoogle verifies rawIDToken (issued client-side by Google
// Identity Services) and returns a real session, in the same shape
// POST /auth/login and POST /auth/signup/verify already produce -- the
// frontend applies the result through the same code path either way. The
// first time a given Google account is seen, this either creates a brand
// new user (no memberships yet, same as a fresh signup -- the existing
// "no memberships -> onboarding" redirect needs no special case for this)
// or, if the email already belongs to a password-based account, silently
// links to it instead: safe specifically because Google, not the caller,
// already verified control of that email.
func (s *Service) SignInWithGoogle(ctx context.Context, rawIDToken, userAgent string, sessionTTL time.Duration) (identity.User, string, string, identity.Session, error) {
	claims, err := s.verifier.Verify(ctx, rawIDToken)
	if err != nil || !claims.EmailVerified {
		return identity.User{}, "", "", identity.Session{}, ErrGoogleSignInFailed
	}

	user, err := s.resolveUser(ctx, claims)
	if err != nil {
		return identity.User{}, "", "", identity.Session{}, err
	}

	rawToken, rawCSRF, sess, err := s.identity.CreateSession(ctx, user.ID, userAgent, sessionTTL)
	if err != nil {
		return identity.User{}, "", "", identity.Session{}, fmt.Errorf("create session: %w", err)
	}
	return user, rawToken, rawCSRF, sess, nil
}

// resolveUser implements the three cases described on SignInWithGoogle:
// already linked, link-to-existing-by-email, or create new.
func (s *Service) resolveUser(ctx context.Context, claims GoogleClaims) (identity.User, error) {
	userID, err := s.lookupLinkedUserID(ctx, claims.Sub)
	if err != nil {
		return identity.User{}, err
	}
	if userID != uuid.Nil {
		return s.identity.GetUserByID(ctx, userID)
	}

	registered, err := s.identity.EmailIsRegistered(ctx, claims.Email)
	if err != nil {
		return identity.User{}, fmt.Errorf("check existing registration: %w", err)
	}

	var user identity.User
	if registered {
		user, err = s.identity.GetUserByEmail(ctx, claims.Email)
	} else {
		user, err = s.identity.CreateUserWithoutPassword(ctx, claims.Email, "", displayNameFromClaims(claims))
	}
	if err != nil {
		return identity.User{}, fmt.Errorf("resolve google account: %w", err)
	}

	// Best-effort, not wrapped in the same transaction as the lookup/create
	// above (each internal/identity call is its own transaction): if this
	// link write fails -- a transient DB error, or a lost race with a
	// concurrent sign-in for the same brand-new Google account -- the
	// account above already exists and this sign-in still succeeds. The
	// next sign-in attempt finds no link yet, re-enters this same branch,
	// and retries the link -- the same non-atomic-but-recoverable shape
	// internal/signup.Service.Verify's own best-effort cleanup step uses.
	_ = s.linkIdentity(ctx, user.ID, claims)
	return user, nil
}

func (s *Service) lookupLinkedUserID(ctx context.Context, providerUserID string) (uuid.UUID, error) {
	var found sqlc.UserIdentity
	err := store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		row, err := q.GetUserIdentity(ctx, sqlc.GetUserIdentityParams{
			Provider: providerGoogle, ProviderUserID: providerUserID,
		})
		if err != nil {
			return err
		}
		found = row
		return nil
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, nil
		}
		return uuid.Nil, fmt.Errorf("look up google identity: %w", err)
	}
	return found.UserID, nil
}

func (s *Service) linkIdentity(ctx context.Context, userID uuid.UUID, claims GoogleClaims) error {
	id, err := newID()
	if err != nil {
		return err
	}
	return store.WithApp(ctx, s.pool, uuid.Nil, func(ctx context.Context, q *sqlc.Queries) error {
		_, err := q.CreateUserIdentity(ctx, sqlc.CreateUserIdentityParams{
			ID: id, UserID: userID, Provider: providerGoogle, ProviderUserID: claims.Sub, Email: claims.Email,
		})
		return err
	})
}

func displayNameFromClaims(claims GoogleClaims) string {
	if claims.Name != "" {
		return claims.Name
	}
	if at := strings.Index(claims.Email, "@"); at > 0 {
		return claims.Email[:at]
	}
	return claims.Email
}
