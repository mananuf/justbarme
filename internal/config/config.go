package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	Development = "development"
	Test        = "test"
	Staging     = "staging"
	Production  = "production"
)

type HTTP struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type Database struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
	HealthTimeout   time.Duration
}

// Session configures the opaque, cookie-carried online session (see
// docs/API_CONTRACT.md §4). It is unrelated to OfflineLease.
type Session struct {
	TTL          time.Duration
	CookieSecure bool
}

// PlatformSession configures the separate session issued to platform staff
// (internal/platformadmin) -- shorter-lived by default than an ordinary
// business Session, since this tier's blast radius if compromised is every
// business, not one. Shares Session.CookieSecure rather than a second
// cookie-security knob, since the two should always match this
// environment's dev/prod posture identically.
type PlatformSession struct {
	TTL time.Duration
}

// Argon2 are the versioned password-hashing cost parameters. Changing these
// only affects newly hashed passwords — internal/auth verifies existing
// hashes using the parameters recorded in the hash itself.
type Argon2 struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
}

// OfflineLease configures the signed, bounded offline authorization lease
// (docs/ARCHITECTURE.md §11.2). PrivateKey and PublicKey are nil when unset;
// callers must decide what that means for their environment (internal/app
// generates an ephemeral development-only keypair rather than failing
// startup outside production).
type OfflineLease struct {
	TTL        time.Duration
	PrivateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

// SMTP configures the transactional-email relay used to deliver signup OTP
// codes (internal/email, internal/signup). The pilot uses Gmail's SMTP
// relay with an app password.
type SMTP struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	// MaxSendAttempts and RetryBaseDelay configure internal/email.WithRetry,
	// which wraps the real SMTP provider (never the console dev fallback,
	// which cannot fail) -- see internal/app.emailProvider.
	MaxSendAttempts int32
	RetryBaseDelay  time.Duration
}

// Configured reports whether real SMTP credentials are present. When
// false, internal/app falls back to email.ConsoleProvider outside
// production (config.Load itself refuses to start in production without
// them).
func (s SMTP) Configured() bool {
	return s.Username != "" && s.Password != "" && s.From != ""
}

// Signup configures the public account-signup flow (internal/signup).
type Signup struct {
	OTPTTL time.Duration
}

// GoogleOAuth configures optional "Sign in with Google" (internal/oauth).
// Unlike SMTP, there is no dev fallback and no production requirement:
// leaving ClientID unset simply leaves POST /auth/google unregistered (see
// internal/app.Run and httpapi.NewHandler) rather than refusing to start,
// since this is an optional extra sign-in method, not core to the pilot.
type GoogleOAuth struct {
	ClientID string
}

type Config struct {
	Env             string
	HTTP            HTTP
	Database        Database
	LogLevel        string
	Session         Session
	PlatformSession PlatformSession
	Argon2          Argon2
	OfflineLease    OfflineLease
	SMTP            SMTP
	Signup          Signup
	GoogleOAuth     GoogleOAuth
	PublicBaseURL   string
}

type lookupEnv func(string) (string, bool)

func Load() (Config, error) {
	return load(os.LookupEnv)
}

func load(lookup lookupEnv) (Config, error) {
	cfg := Config{
		Env: Development,
		HTTP: HTTP{
			Addr:              ":8080",
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
			ShutdownTimeout:   10 * time.Second,
		},
		Database: Database{
			MaxConns:        10,
			MinConns:        1,
			MaxConnLifetime: 30 * time.Minute,
			MaxConnIdleTime: 5 * time.Minute,
			HealthTimeout:   2 * time.Second,
		},
		LogLevel: "info",
		Session: Session{
			TTL:          720 * time.Hour,
			CookieSecure: true,
		},
		PlatformSession: PlatformSession{
			TTL: 12 * time.Hour,
		},
		Argon2: Argon2{
			MemoryKiB:   64 * 1024,
			Iterations:  3,
			Parallelism: 2,
		},
		OfflineLease: OfflineLease{
			TTL: 168 * time.Hour,
		},
		SMTP: SMTP{
			Host:            "smtp.gmail.com",
			Port:            "587",
			MaxSendAttempts: 3,
			RetryBaseDelay:  500 * time.Millisecond,
		},
		Signup: Signup{
			OTPTTL: 10 * time.Minute,
		},
	}

	var problems []string

	cfg.Env = stringValue(lookup, "JBM_ENV", cfg.Env)
	if !oneOf(cfg.Env, Development, Test, Staging, Production) {
		problems = append(problems, "JBM_ENV must be one of development, test, staging, or production")
	}

	cfg.HTTP.Addr = stringValue(lookup, "JBM_HTTP_ADDR", cfg.HTTP.Addr)
	if strings.TrimSpace(cfg.HTTP.Addr) == "" {
		problems = append(problems, "JBM_HTTP_ADDR must not be empty")
	}

	parseDuration(lookup, "JBM_HTTP_READ_HEADER_TIMEOUT", &cfg.HTTP.ReadHeaderTimeout, &problems)
	parseDuration(lookup, "JBM_HTTP_READ_TIMEOUT", &cfg.HTTP.ReadTimeout, &problems)
	parseDuration(lookup, "JBM_HTTP_WRITE_TIMEOUT", &cfg.HTTP.WriteTimeout, &problems)
	parseDuration(lookup, "JBM_HTTP_IDLE_TIMEOUT", &cfg.HTTP.IdleTimeout, &problems)
	parseDuration(lookup, "JBM_HTTP_SHUTDOWN_TIMEOUT", &cfg.HTTP.ShutdownTimeout, &problems)

	if value, ok := lookup("JBM_DATABASE_URL"); ok {
		cfg.Database.URL = strings.TrimSpace(value)
	}
	if cfg.Database.URL == "" {
		problems = append(problems, "JBM_DATABASE_URL is required")
	}

	parseInt32(lookup, "JBM_DB_MAX_CONNS", &cfg.Database.MaxConns, false, &problems)
	parseInt32(lookup, "JBM_DB_MIN_CONNS", &cfg.Database.MinConns, true, &problems)
	parseDuration(lookup, "JBM_DB_MAX_CONN_LIFETIME", &cfg.Database.MaxConnLifetime, &problems)
	parseDuration(lookup, "JBM_DB_MAX_CONN_IDLE_TIME", &cfg.Database.MaxConnIdleTime, &problems)
	parseDuration(lookup, "JBM_DB_HEALTH_TIMEOUT", &cfg.Database.HealthTimeout, &problems)

	if cfg.Database.MinConns > cfg.Database.MaxConns {
		problems = append(problems, "JBM_DB_MIN_CONNS must not exceed JBM_DB_MAX_CONNS")
	}

	cfg.LogLevel = strings.ToLower(stringValue(lookup, "JBM_LOG_LEVEL", cfg.LogLevel))
	if !oneOf(cfg.LogLevel, "debug", "info", "warn", "error") {
		problems = append(problems, "JBM_LOG_LEVEL must be one of debug, info, warn, or error")
	}

	parseDuration(lookup, "JBM_SESSION_TTL", &cfg.Session.TTL, &problems)
	parseBool(lookup, "JBM_SESSION_COOKIE_SECURE", &cfg.Session.CookieSecure, &problems)
	if oneOf(cfg.Env, Staging, Production) && !cfg.Session.CookieSecure {
		problems = append(problems, "JBM_SESSION_COOKIE_SECURE must be true in staging or production")
	}

	parseDuration(lookup, "JBM_PLATFORM_SESSION_TTL", &cfg.PlatformSession.TTL, &problems)

	parseUint32(lookup, "JBM_ARGON2_MEMORY_KIB", &cfg.Argon2.MemoryKiB, &problems)
	parseUint32(lookup, "JBM_ARGON2_ITERATIONS", &cfg.Argon2.Iterations, &problems)
	parseUint8(lookup, "JBM_ARGON2_PARALLELISM", &cfg.Argon2.Parallelism, &problems)

	parseDuration(lookup, "JBM_OFFLINE_LEASE_TTL", &cfg.OfflineLease.TTL, &problems)
	parseOfflineSigningKeys(lookup, &cfg.OfflineLease, &problems)
	if cfg.Env == Production && (cfg.OfflineLease.PrivateKey == nil || cfg.OfflineLease.PublicKey == nil) {
		problems = append(problems, "JBM_OFFLINE_SIGNING_PRIVATE_KEY and JBM_OFFLINE_SIGNING_PUBLIC_KEY are required in production")
	}

	cfg.SMTP.Host = stringValue(lookup, "JBM_SMTP_HOST", cfg.SMTP.Host)
	cfg.SMTP.Port = stringValue(lookup, "JBM_SMTP_PORT", cfg.SMTP.Port)
	cfg.SMTP.Username = stringValue(lookup, "JBM_SMTP_USERNAME", cfg.SMTP.Username)
	cfg.SMTP.Password = stringValue(lookup, "JBM_SMTP_PASSWORD", cfg.SMTP.Password)
	cfg.SMTP.From = stringValue(lookup, "JBM_SMTP_FROM", cfg.SMTP.From)
	if cfg.Env == Production && !cfg.SMTP.Configured() {
		problems = append(problems, "JBM_SMTP_USERNAME, JBM_SMTP_PASSWORD, and JBM_SMTP_FROM are required in production")
	}
	parseInt32(lookup, "JBM_SMTP_MAX_SEND_ATTEMPTS", &cfg.SMTP.MaxSendAttempts, false, &problems)
	parseDuration(lookup, "JBM_SMTP_RETRY_BASE_DELAY", &cfg.SMTP.RetryBaseDelay, &problems)

	parseDuration(lookup, "JBM_SIGNUP_OTP_TTL", &cfg.Signup.OTPTTL, &problems)

	cfg.GoogleOAuth.ClientID = stringValue(lookup, "JBM_GOOGLE_OAUTH_CLIENT_ID", cfg.GoogleOAuth.ClientID)

	cfg.PublicBaseURL = stringValue(lookup, "JBM_PUBLIC_BASE_URL", cfg.PublicBaseURL)
	if cfg.Env == Production && !strings.HasPrefix(cfg.PublicBaseURL, "https://") {
		problems = append(problems, "JBM_PUBLIC_BASE_URL must be an https URL in production")
	}

	if len(problems) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %s", strings.Join(problems, "; "))
	}

	return cfg, nil
}

func stringValue(lookup lookupEnv, key, fallback string) string {
	if value, ok := lookup(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func parseDuration(lookup lookupEnv, key string, target *time.Duration, problems *[]string) {
	value, ok := lookup(key)
	if !ok {
		return
	}

	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || duration <= 0 {
		*problems = append(*problems, key+" must be a positive duration such as 5s or 2m")
		return
	}
	*target = duration
}

func parseInt32(lookup lookupEnv, key string, target *int32, allowZero bool, problems *[]string) {
	value, ok := lookup(key)
	if !ok {
		return
	}

	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
	invalid := err != nil || parsed < 0 || (!allowZero && parsed == 0)
	if invalid {
		qualifier := "a positive integer"
		if allowZero {
			qualifier = "a non-negative integer"
		}
		*problems = append(*problems, key+" must be "+qualifier)
		return
	}
	*target = int32(parsed)
}

func parseBool(lookup lookupEnv, key string, target *bool, problems *[]string) {
	value, ok := lookup(key)
	if !ok {
		return
	}

	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		*problems = append(*problems, key+" must be true or false")
		return
	}
	*target = parsed
}

func parseUint32(lookup lookupEnv, key string, target *uint32, problems *[]string) {
	value, ok := lookup(key)
	if !ok {
		return
	}

	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
	if err != nil || parsed == 0 {
		*problems = append(*problems, key+" must be a positive integer")
		return
	}
	*target = uint32(parsed)
}

func parseUint8(lookup lookupEnv, key string, target *uint8, problems *[]string) {
	value, ok := lookup(key)
	if !ok {
		return
	}

	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 8)
	if err != nil || parsed == 0 {
		*problems = append(*problems, key+" must be a positive integer no greater than 255")
		return
	}
	*target = uint8(parsed)
}

// parseOfflineSigningKeys decodes the base64-encoded Ed25519 keypair used to
// sign offline authorization leases, if both are present. It is deliberately
// strict: a partially configured pair, malformed base64, wrong key length,
// or a public key that does not match the private key are all reported as
// problems rather than silently falling back to no signing key.
func parseOfflineSigningKeys(lookup lookupEnv, target *OfflineLease, problems *[]string) {
	rawPrivate, hasPrivate := lookup("JBM_OFFLINE_SIGNING_PRIVATE_KEY")
	rawPublic, hasPublic := lookup("JBM_OFFLINE_SIGNING_PUBLIC_KEY")
	rawPrivate, rawPublic = strings.TrimSpace(rawPrivate), strings.TrimSpace(rawPublic)

	if (!hasPrivate || rawPrivate == "") && (!hasPublic || rawPublic == "") {
		return
	}
	if rawPrivate == "" || rawPublic == "" {
		*problems = append(*problems, "JBM_OFFLINE_SIGNING_PRIVATE_KEY and JBM_OFFLINE_SIGNING_PUBLIC_KEY must both be set, or both left empty")
		return
	}

	privateKey, err := base64.StdEncoding.DecodeString(rawPrivate)
	if err != nil || len(privateKey) != ed25519.PrivateKeySize {
		*problems = append(*problems, "JBM_OFFLINE_SIGNING_PRIVATE_KEY must be a base64-encoded 64-byte Ed25519 private key")
		return
	}
	publicKey, err := base64.StdEncoding.DecodeString(rawPublic)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		*problems = append(*problems, "JBM_OFFLINE_SIGNING_PUBLIC_KEY must be a base64-encoded 32-byte Ed25519 public key")
		return
	}

	derivedPublic := ed25519.PrivateKey(privateKey).Public().(ed25519.PublicKey)
	if !derivedPublic.Equal(ed25519.PublicKey(publicKey)) {
		*problems = append(*problems, "JBM_OFFLINE_SIGNING_PUBLIC_KEY does not match JBM_OFFLINE_SIGNING_PRIVATE_KEY")
		return
	}

	target.PrivateKey = ed25519.PrivateKey(privateKey)
	target.PublicKey = ed25519.PublicKey(publicKey)
}

func oneOf(value string, permitted ...string) bool {
	for _, candidate := range permitted {
		if value == candidate {
			return true
		}
	}
	return false
}
