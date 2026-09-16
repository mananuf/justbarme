package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/email"
	"github.com/mananuf/justbarme/internal/httpapi"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/inventory"
	"github.com/mananuf/justbarme/internal/oauth"
	"github.com/mananuf/justbarme/internal/platformadmin"
	"github.com/mananuf/justbarme/internal/sales"
	"github.com/mananuf/justbarme/internal/signup"
	"github.com/mananuf/justbarme/internal/store"
)

func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg, os.Stdout)
	pool, err := store.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := ensureOfflineSigningKeys(&cfg, logger); err != nil {
		return err
	}

	identitySvc := identity.New(pool, cfg.Argon2)
	catalogueSvc := catalogue.New(pool)
	inventorySvc := inventory.New(pool)
	salesSvc := sales.New(pool)
	platformAdminSvc := platformadmin.New(pool, cfg.Argon2)
	signupSvc := signup.New(pool, cfg.Argon2, identitySvc, emailProvider(cfg, logger), cfg.Signup.OTPTTL)
	oauthSvc := googleOAuthService(cfg, pool, identitySvc, logger)

	handler := httpapi.NewHandler(httpapi.Dependencies{
		Logger:        logger,
		Database:      pool,
		HealthTimeout: cfg.Database.HealthTimeout,
		Version:       Version,
		Identity:      identitySvc,
		Catalogue:     catalogueSvc,
		Inventory:     inventorySvc,
		Sales:         salesSvc,
		PlatformAdmin: platformAdminSvc,
		Signup:        signupSvc,
		OAuth:         oauthSvc,
		Config:        cfg,
	})
	server := httpapi.NewServer(cfg.HTTP, handler, logger)

	logger.Info("server starting", "addr", cfg.HTTP.Addr, "environment", cfg.Env, "version", Version)
	if err := runServer(ctx, server, cfg.HTTP.ShutdownTimeout); err != nil {
		return err
	}
	logger.Info("server stopped", "addr", cfg.HTTP.Addr)
	return nil
}

// ensureOfflineSigningKeys generates a throwaway Ed25519 keypair when none
// is configured outside production. config.Load already refuses to start in
// production without JBM_OFFLINE_SIGNING_PRIVATE_KEY/PUBLIC_KEY set, so this
// only ever fires for local development and tests — a restart invalidates
// every previously issued offline lease, which is expected and harmless
// there.
func ensureOfflineSigningKeys(cfg *config.Config, logger *slog.Logger) error {
	if cfg.OfflineLease.PrivateKey != nil {
		return nil
	}

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate ephemeral offline signing keypair: %w", err)
	}
	cfg.OfflineLease.PrivateKey = private
	cfg.OfflineLease.PublicKey = public

	logger.Warn("no offline signing keys configured; generated an ephemeral development-only keypair",
		"note", "every offline lease issued this run becomes invalid on restart; set JBM_OFFLINE_SIGNING_PRIVATE_KEY/PUBLIC_KEY to persist across restarts")
	return nil
}

// emailProvider returns a real SMTP provider, wrapped with retry/backoff,
// when configured, or a console-logging one outside production when it
// isn't. config.Load already refuses to start in production without SMTP
// credentials, so the console fallback only ever fires in development/test
// -- the same dev-convenience-not-silent-gap pattern as
// ensureOfflineSigningKeys. The retry wrapper is only applied to the real
// SMTP provider: ConsoleProvider never fails, so retrying it would just be
// dead code exercised on every call.
func emailProvider(cfg config.Config, logger *slog.Logger) email.Provider {
	if cfg.SMTP.Configured() {
		smtp := email.NewSMTPProvider(email.SMTPConfig{
			Host: cfg.SMTP.Host, Port: cfg.SMTP.Port,
			Username: cfg.SMTP.Username, Password: cfg.SMTP.Password, From: cfg.SMTP.From,
		})
		return email.WithRetry(smtp, email.RetryConfig{
			MaxAttempts: int(cfg.SMTP.MaxSendAttempts),
			BaseDelay:   cfg.SMTP.RetryBaseDelay,
		})
	}
	logger.Warn("no SMTP credentials configured; signup codes will only be logged, not emailed",
		"note", "set JBM_SMTP_USERNAME/PASSWORD/FROM to send real email")
	return email.NewConsoleProvider(logger)
}

// googleOAuthService returns nil when JBM_GOOGLE_OAUTH_CLIENT_ID is unset --
// unlike email, there is no dev-fallback provider for "sign in with an
// external identity provider," so the feature is simply absent rather than
// faked. httpapi.NewHandler only registers POST /auth/google when this is
// non-nil.
func googleOAuthService(cfg config.Config, pool *pgxpool.Pool, identitySvc *identity.Service, logger *slog.Logger) *oauth.Service {
	if cfg.GoogleOAuth.ClientID == "" {
		logger.Warn("no Google OAuth client ID configured; Google sign-in is disabled",
			"note", "set JBM_GOOGLE_OAUTH_CLIENT_ID to enable it")
		return nil
	}
	return oauth.New(pool, identitySvc, oauth.NewGoogleVerifier(cfg.GoogleOAuth.ClientID))
}

type server interface {
	ListenAndServe() error
	Shutdown(context.Context) error
}

func runServer(ctx context.Context, srv server, shutdownTimeout time.Duration) error {
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}

	if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP after shutdown: %w", err)
	}
	return nil
}
