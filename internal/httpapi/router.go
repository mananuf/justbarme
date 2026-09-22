package httpapi

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mananuf/justbarme/internal/activity"
	"github.com/mananuf/justbarme/internal/auth"
	"github.com/mananuf/justbarme/internal/business"
	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/expenses"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/inventory"
	"github.com/mananuf/justbarme/internal/invitations"
	"github.com/mananuf/justbarme/internal/metrics"
	"github.com/mananuf/justbarme/internal/oauth"
	"github.com/mananuf/justbarme/internal/platformadmin"
	"github.com/mananuf/justbarme/internal/reports"
	"github.com/mananuf/justbarme/internal/sales"
	"github.com/mananuf/justbarme/internal/signup"
	"github.com/mananuf/justbarme/internal/verification"
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
	Inventory     *inventory.Service
	Sales         *sales.Service
	Expenses      *expenses.Service
	Activity      *activity.Service
	Reports       *reports.Service
	Business      *business.Service
	PlatformAdmin *platformadmin.Service
	// Metrics is nil-safe -- accessLogMiddleware and the login handler
	// simply skip recording when it's unset (e.g. a test harness that
	// doesn't care about metrics).
	Metrics      *metrics.Registry
	Signup       *signup.Service
	Invitations  *invitations.Service
	Verification *verification.Service
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

	identity          *identity.Service
	catalogue         *catalogue.Service
	inventory         *inventory.Service
	sales             *sales.Service
	expenses          *expenses.Service
	activity          *activity.Service
	reports           *reports.Service
	business          *business.Service
	platformAdmin     *platformadmin.Service
	metrics           *metrics.Registry
	signup            *signup.Service
	invitations       *invitations.Service
	verification      *verification.Service
	oauth             *oauth.Service
	env               string
	zavuWebhookSecret string

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

	invitationCreateBusinessLimiter   *auth.Limiter
	invitationCreateIPLimiter         *auth.Limiter
	invitationLookupIPLimiter         *auth.Limiter
	invitationAcceptIdentifierLimiter *auth.Limiter
	invitationAcceptIPLimiter         *auth.Limiter

	googleSignInIPLimiter *auth.Limiter

	publicBillLookupIPLimiter *auth.Limiter

	metricsToken string
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

		identity:          deps.Identity,
		catalogue:         deps.Catalogue,
		inventory:         deps.Inventory,
		sales:             deps.Sales,
		expenses:          deps.Expenses,
		activity:          deps.Activity,
		reports:           deps.Reports,
		business:          deps.Business,
		platformAdmin:     deps.PlatformAdmin,
		metrics:           deps.Metrics,
		metricsToken:      deps.Config.MetricsToken,
		signup:            deps.Signup,
		invitations:       deps.Invitations,
		verification:      deps.Verification,
		oauth:             deps.OAuth,
		env:               deps.Config.Env,
		zavuWebhookSecret: deps.Config.Zavu.WebhookSecret,

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

		// Invitations (docs/PHASE_INVITATIONS_WHATSAPP.md §8): creation is
		// looser than signup-start's 3/hour-per-email since one owner
		// legitimately inviting several staff in a burst (a shift
		// changeover, a new location) isn't abuse. Lookup/accept mirror
		// signup-verify's exact numbers -- the per-row attempt lockout in
		// internal/verification/internal/signup is the real brute-force
		// defense; these are the same coarser, second-layer shape.
		invitationCreateBusinessLimiter:   auth.NewLimiter(10, time.Hour),
		invitationCreateIPLimiter:         auth.NewLimiter(30, time.Hour),
		invitationLookupIPLimiter:         auth.NewLimiter(30, 15*time.Minute),
		invitationAcceptIdentifierLimiter: auth.NewLimiter(10, 15*time.Minute),
		invitationAcceptIPLimiter:         auth.NewLimiter(30, 15*time.Minute),

		// Cryptographic ID-token verification isn't guessable the way a
		// 6-digit OTP is, so this only needs to bound wasted verification
		// work from garbage tokens, not defend against brute-forcing --
		// generous enough that legitimate multi-tab sign-ins never trip it.
		googleSignInIPLimiter: auth.NewLimiter(30, time.Minute),

		// docs/PHASE_PILOT_RELEASE.md §4: a genuinely unauthenticated
		// surface next to a guessable-adjacent token, same shape and
		// numbers as invitationLookupIPLimiter above.
		publicBillLookupIPLimiter: auth.NewLimiter(30, 15*time.Minute),
	}

	router := chi.NewRouter()
	router.Use(api.requestIDMiddleware)
	router.Use(api.accessLogMiddleware)
	router.Use(api.recoverPanicMiddleware)
	router.Use(securityHeadersMiddleware)

	router.NotFound(api.notFoundResponse)
	router.MethodNotAllowed(api.methodNotAllowedResponse)

	// Outside production only: serves whatever storage.LocalDiskProvider
	// wrote to disk (see internal/app.storageProvider) so an uploaded logo
	// is genuinely viewable in development without real object-storage
	// credentials. Never registered in production -- config.Load refuses
	// to start there without a real provider configured, so this path
	// would just be dead weight.
	if api.env != config.Production && deps.Config.Storage.LocalDir != "" {
		fileServer := http.FileServer(http.Dir(deps.Config.Storage.LocalDir))
		router.Handle("/dev-uploads/*", http.StripPrefix("/dev-uploads/", fileServer))
	}

	// Deliberately outside /api/v1 -- Prometheus scrapers expect a bare
	// /metrics path, and this isn't a versioned application API surface.
	// See metricsHandler for the optional bearer-token gate.
	router.Get("/metrics", api.metricsHandler)

	router.Route("/api/v1", func(router chi.Router) {
		router.Get("/health/live", api.liveness)
		router.Get("/health/ready", api.readiness)

		router.Post("/auth/login", api.login)
		router.Post("/auth/signup/start", api.signupStart)
		router.Post("/auth/signup/verify", api.signupVerify)
		if api.oauth != nil {
			router.Post("/auth/google", api.googleSignIn)
		}

		// Public: an invitee has no session and no business context yet --
		// the hashed token itself is the access control (see migration
		// 000021's reasoning for why invitations carries no RLS).
		router.Get("/invitations/{token}", api.getInvitationByToken)
		router.Post("/invitations/{token}/accept", api.acceptInvitationAsNewUser)

		router.Post("/webhooks/zavu", api.zavuWebhook)

		// Public, unauthenticated: a bill-share-link recipient has no
		// session and no business context -- the row's own unguessable
		// hashed token is the access control, same reasoning as the
		// invitation lookup above (docs/PHASE_PILOT_RELEASE.md §4).
		router.Get("/public/bills/{token}", api.getPublicBill)

		router.Group(func(router chi.Router) {
			router.Use(api.requireAuth)
			router.Use(api.requireCSRF)

			router.Post("/auth/logout", api.logout)
			router.Get("/me", api.me)
			router.Post("/businesses", api.createBusiness)
			router.Get("/catalogue-templates", api.listCatalogueTemplates)

			// Authenticated but deliberately not business-scoped: accepting
			// an invitation to a DIFFERENT business than whichever one is
			// currently selected must not require that business already
			// being the active tenant context, and identity verification
			// (docs/PHASE_INVITATIONS_WHATSAPP.md's verify-before-link) is
			// a user-level action, not a business-level one.
			router.Post("/invitations/{token}/link", api.linkInvitationToExistingUser)
			router.Post("/identity-verifications/{verification_id}/confirm", api.confirmIdentityVerification)
			router.Post("/me/identifiers", api.startAddIdentifier)

			router.Group(func(router chi.Router) {
				router.Use(api.requireBusinessContext)

				router.Get("/business", api.getBusiness)
				router.Patch("/business", api.updateBusinessSettings)
				router.Post("/business/logo", api.uploadBusinessLogo)

				router.Post("/invitations", api.createInvitation)
				router.Get("/invitations", api.listInvitations)
				router.Post("/invitations/{invitation_id}/revoke", api.revokeInvitation)

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

				router.Post("/stock-receipts", api.receiveStock)
				router.Get("/products/variants/{variant_id}/history", api.getInventoryHistory)

				router.Post("/stock-counts", api.submitStockCount)

				router.Post("/inventory-adjustments", api.requestAdjustment)
				router.Get("/inventory-adjustments", api.listPendingAdjustments)
				router.Post("/inventory-adjustments/{adjustment_id}/approve", api.approveAdjustment)
				router.Post("/inventory-adjustments/{adjustment_id}/reject", api.rejectAdjustment)

				router.Get("/inventory-reviews", api.listInventoryReviews)
				router.Post("/inventory-reviews/{review_id}/resolve", api.resolveInventoryReview)

				router.Post("/sales", api.createSale)
				router.Get("/sales", api.listSales)
				router.Get("/sales/summary", api.salesSummary)
				router.Post("/sales/{sale_id}/reverse", api.reverseSale)
				router.Get("/sale-reviews", api.listSaleReviews)
				router.Post("/sale-reviews/{review_id}/resolve", api.resolveSaleReview)

				router.Get("/expense-categories", api.listExpenseCategories)
				router.Post("/expense-categories", api.createExpenseCategory)
				router.Post("/expenses", api.recordExpense)
				router.Get("/expenses", api.listExpenses)
				router.Post("/expenses/{expense_id}/reverse", api.reverseExpense)

				router.Get("/activity", api.listActivity)
				router.Get("/activity/heatmap", api.activityHeatmap)

				router.Get("/reports/sales", api.reportSales)
				router.Get("/reports/products", api.reportProducts)
				router.Get("/reports/staff-sales", api.reportStaffSales)
				router.Get("/reports/expenses", api.reportExpenses)
				router.Get("/reports/stock", api.reportStock)
				router.Get("/reports/stock-purchases", api.reportStockPurchases)
				router.Get("/reports/gross-margin", api.reportGrossMargin)

				router.Get("/dashboard", api.getDashboard)

				router.Get("/tables", api.listTables)
				router.Post("/tables", api.createTable)

				router.Get("/customers", api.listCustomers)
				router.Post("/customers", api.createCustomer)

				router.Post("/bills", api.openBill)
				router.Get("/bills", api.listBills)
				router.Get("/bills/{bill_id}", api.getBillDetail)
				router.Post("/bills/{bill_id}/rounds", api.addSaleRound)
				router.Post("/bills/{bill_id}/items/remove", api.removeBillItem)
				router.Post("/bills/{bill_id}/close", api.closeBill)
				router.Post("/bills/{bill_id}/void", api.voidBill)
				router.Post("/bills/{bill_id}/write-off", api.writeOffBill)
				router.Post("/bills/{bill_id}/payments", api.recordPayment)
				router.Post("/payments/{payment_id}/reverse", api.reversePayment)

				router.Post("/bills/{bill_id}/share-link", api.createOrRotateBillShareLink)
				router.Post("/bills/{bill_id}/share-link/revoke", api.revokeBillShareLink)
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
