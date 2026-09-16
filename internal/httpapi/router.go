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
	"github.com/mananuf/justbarme/internal/platformadmin"
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
	Config        config.Config
}

type API struct {
	logger        *slog.Logger
	database      DatabasePinger
	healthTimeout time.Duration
	version       string

	identity      *identity.Service
	catalogue     *catalogue.Service
	platformAdmin *platformadmin.Service
	env           string

	sessionTTL               time.Duration
	sessionCookieSecure      bool
	offlineLeaseTTL          time.Duration
	offlineSigningPrivateKey ed25519.PrivateKey
	platformSessionTTL       time.Duration

	loginLimiter         *auth.Limiter
	platformLoginLimiter *auth.Limiter
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
