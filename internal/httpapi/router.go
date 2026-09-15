package httpapi

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mananuf/justbarme/internal/auth"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/identity"
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
	Config        config.Config
}

type API struct {
	logger        *slog.Logger
	database      DatabasePinger
	healthTimeout time.Duration
	version       string

	identity *identity.Service
	env      string

	sessionTTL               time.Duration
	sessionCookieSecure      bool
	offlineLeaseTTL          time.Duration
	offlineSigningPrivateKey ed25519.PrivateKey

	loginLimiter *auth.Limiter
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

		identity: deps.Identity,
		env:      deps.Config.Env,

		sessionTTL:               deps.Config.Session.TTL,
		sessionCookieSecure:      deps.Config.Session.CookieSecure,
		offlineLeaseTTL:          deps.Config.OfflineLease.TTL,
		offlineSigningPrivateKey: deps.Config.OfflineLease.PrivateKey,

		// Five attempts per email per minute: enough for a genuine typo,
		// tight enough to blunt credential stuffing against one account.
		// See docs/IMPLEMENTATION_PLAN.md — a process-local limiter is
		// accepted only for a single-instance pilot deployment.
		loginLimiter: auth.NewLimiter(5, time.Minute),
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

			router.Group(func(router chi.Router) {
				router.Use(api.requireBusinessContext)

				router.Get("/business", api.getBusiness)
				router.Post("/devices/enroll", api.enrollDevice)
				router.Get("/devices", api.listDevices)
				router.Delete("/devices/{device_id}", api.revokeDevice)
			})
		})
	})

	return router
}
