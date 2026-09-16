package httpapi

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mananuf/justbarme/internal/auth"
	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/oauth"
	"github.com/mananuf/justbarme/internal/platformadmin"
	"github.com/mananuf/justbarme/internal/signup"
)

type DatabasePinger interface {
	Ping(context.Context) error
}

type Dependencies struct {
	Logger        *slog.Logger
	Database      DatabasePinger
	HealthTimeout time.Duration
	Version       string
	Identity      *identity.Service
	Catalogue     *catalogue.Service
	PlatformAdmin *platformadmin.Service
	Signup        *signup.Service
	// OAuth is nil when Google sign-in isn't configured (see
	// internal/app.googleOAuthService) -- NewHandler only registers
	// POST /auth/google when it's set.
	OAuth  *oauth.Service
	Config config.Config
}

type API struct {
	logger        *slog.Logger
	database      DatabasePinger
	healthTimeout time.Duration
	version       string

	identity      *identity.Service
	catalogue     *catalogue.Service
	platformAdmin *platformadmin.Service
	signup        *signup.Service
	oauth         *oauth.Service
	env           string

	sessionTTL               time.Duration
	sessionCookieSecure      bool
	offlineLeaseTTL          time.Duration
	offlineSigningPrivateKey ed25519.PrivateKey
	platformSessionTTL       time.Duration

	loginLimiter         *auth.Limiter
	platformLoginLimiter *auth.Limiter

	signupStartEmailLimiter  *auth.Limiter
	signupStartIPLimiter     *auth.Limiter
	signupVerifyEmailLimiter *auth.Limiter
	signupVerifyIPLimiter    *auth.Limiter

	googleSignInIPLimiter *auth.Limiter
}

func NewHandler(deps Dependencies) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.HealthTimeout <= 0 {
		deps.HealthTimeout = 2 * time.Second
	}
	if deps.Version == "" {
		deps.Version = "dev"
	}

	api := &API{
		logger:        deps.Logger,
		database:      deps.Database,
		healthTimeout: deps.HealthTimeout,
		version:       deps.Version,

		identity:      deps.Identity,
		catalogue:     deps.Catalogue,
		platformAdmin: deps.PlatformAdmin,
		signup:        deps.Signup,
		oauth:         deps.OAuth,
		env:           deps.Config.Env,

		sessionTTL:               deps.Config.Session.TTL,
		sessionCookieSecure:      deps.Config.Session.CookieSecure,
		offlineLeaseTTL:          deps.Config.OfflineLease.TTL,
		offlineSigningPrivateKey: deps.Config.OfflineLease.PrivateKey,
		platformSessionTTL:       deps.Config.PlatformSession.TTL,

		// Five attempts per email per minute: enough for a genuine typo,
		// tight enough to blunt credential stuffing against one account.
		// See docs/IMPLEMENTATION_PLAN.md — a process-local limiter is
		// accepted only for a single-instance pilot deployment.
		loginLimiter: auth.NewLimiter(5, time.Minute),
		// Tighter than the business login limiter: this tier's blast
		// radius if compromised is every business, not one.
		platformLoginLimiter: auth.NewLimiter(5, 15*time.Minute),

		// Signup is rate-limited on both axes: per email (a spammed inbox
		// is direct, visible harm from a typo'd or malicious target
		// address) and per IP (this endpoint creates database rows and
		// sends real email, a more attractive abuse target than login).
		signupStartEmailLimiter: auth.NewLimiter(3, time.Hour),
		signupStartIPLimiter:    auth.NewLimiter(10, time.Hour),
		// Verify has its own per-row attempts counter (internal/signup's
		// maxOTPAttempts) as the primary brute-force defense; these are a
		// second, coarser layer against hammering many different emails
		// from one source or one email from many sources.
		signupVerifyEmailLimiter: auth.NewLimiter(10, 15*time.Minute),
		signupVerifyIPLimiter:    auth.NewLimiter(30, 15*time.Minute),

		// Cryptographic ID-token verification isn't guessable the way a
		// 6-digit OTP is, so this only needs to bound wasted verification
		// work from garbage tokens, not defend against brute-forcing --
		// generous enough that legitimate multi-tab sign-ins never trip it.
		googleSignInIPLimiter: auth.NewLimiter(30, time.Minute),
	}

	router := chi.NewRouter()
	router.Use(api.requestIDMiddleware)
	router.Use(api.accessLogMiddleware)
	router.Use(api.recoverPanicMiddleware)
	router.Use(securityHeadersMiddleware)

	router.NotFound(api.notFoundResponse)
	router.MethodNotAllowed(api.methodNotAllowedResponse)

	router.Route("/api/v1", func(router chi.Router) {
		router.Get("/health/live", api.liveness)
		router.Get("/health/ready", api.readiness)

		router.Post("/auth/login", api.login)
		router.Post("/auth/signup/start", api.signupStart)
		router.Post("/auth/signup/verify", api.signupVerify)
		if api.oauth != nil {
			router.Post("/auth/google", api.googleSignIn)
		}

		router.Group(func(router chi.Router) {
			router.Use(api.requireAuth)
			router.Use(api.requireCSRF)

			router.Post("/auth/logout", api.logout)
			router.Get("/me", api.me)
			router.Post("/businesses", api.createBusiness)
			router.Get("/catalogue-templates", api.listCatalogueTemplates)

			router.Group(func(router chi.Router) {
				router.Use(api.requireBusinessContext)

				router.Get("/business", api.getBusiness)
				router.Post("/devices/enroll", api.enrollDevice)
				router.Get("/devices", api.listDevices)
				router.Delete("/devices/{device_id}", api.revokeDevice)

				router.Post("/catalogue-templates/apply", api.applyCatalogueTemplates)

				router.Get("/categories", api.listCategories)
				router.Post("/categories", api.createCategory)
				router.Patch("/categories/{category_id}", api.updateCategory)

				router.Get("/products", api.listProducts)
				router.Post("/products", api.createProduct)
				router.Get("/products/{product_id}", api.getProduct)
				router.Patch("/products/{product_id}", api.updateProduct)
				router.Post("/products/{product_id}/variants", api.createVariant)

				router.Patch("/variants/{variant_id}", api.updateVariant)
				router.Get("/variants/{variant_id}/prices", api.listVariantPrices)
				router.Post("/variants/{variant_id}/prices", api.setVariantPrice)
			})
		})

		// Platform staff oversee businesses across the whole service. A
		// wholly separate auth surface from everything above: different
		// cookie, different session table, no X-Business-ID or
		// tenancy.Principal anywhere in this group. See CLAUDE.md's
		// platform admin section for why this is intentionally minimal
		// (no cross-tenant data access, no RLS bypass).
		router.Route("/platform", func(router chi.Router) {
			router.Post("/auth/login", api.platformLogin)

			router.Group(func(router chi.Router) {
				router.Use(api.requirePlatformAuth)
				router.Use(api.requirePlatformCSRF)

				router.Post("/auth/logout", api.platformLogout)
				router.Get("/me", api.platformMe)

				router.Get("/businesses", api.listPlatformBusinesses)
				router.Post("/businesses/{business_id}/suspend", api.suspendPlatformBusiness)
				router.Post("/businesses/{business_id}/reactivate", api.reactivatePlatformBusiness)

				router.Get("/audit-log", api.listPlatformAuditLog)
			})
		})
	})

	return router
}
