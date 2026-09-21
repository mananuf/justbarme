package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(mapLookup(map[string]string{"JBM_DATABASE_URL": "postgres://localhost/justbarme"}))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}

	if cfg.Env != Development || cfg.HTTP.Addr != ":8080" || cfg.LogLevel != "info" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.HTTP.ReadHeaderTimeout != 5*time.Second || cfg.HTTP.ReadTimeout != 10*time.Second || cfg.HTTP.WriteTimeout != 15*time.Second || cfg.HTTP.IdleTimeout != 60*time.Second || cfg.HTTP.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected HTTP defaults: %+v", cfg.HTTP)
	}
	if cfg.Database.MaxConns != 10 || cfg.Database.MinConns != 1 || cfg.Database.MaxConnLifetime != 30*time.Minute || cfg.Database.MaxConnIdleTime != 5*time.Minute || cfg.Database.HealthTimeout != 2*time.Second {
		t.Fatalf("unexpected database defaults: %+v", cfg.Database)
	}
	if cfg.Session.TTL != 720*time.Hour || !cfg.Session.CookieSecure {
		t.Fatalf("unexpected session defaults: %+v", cfg.Session)
	}
	if cfg.PlatformSession.TTL != 12*time.Hour {
		t.Fatalf("unexpected platform session TTL default: %v", cfg.PlatformSession.TTL)
	}
	if cfg.SMTP.Host != "smtp.gmail.com" || cfg.SMTP.Port != "587" || cfg.SMTP.Configured() {
		t.Fatalf("unexpected SMTP defaults: %+v", cfg.SMTP)
	}
	if cfg.SMTP.MaxSendAttempts != 3 || cfg.SMTP.RetryBaseDelay != 500*time.Millisecond {
		t.Fatalf("unexpected SMTP retry defaults: %+v", cfg.SMTP)
	}
	if cfg.Zavu.Configured() {
		t.Fatalf("unexpected Zavu defaults: %+v", cfg.Zavu)
	}
	if cfg.Zavu.MaxSendAttempts != 3 || cfg.Zavu.RetryBaseDelay != 500*time.Millisecond {
		t.Fatalf("unexpected Zavu retry defaults: %+v", cfg.Zavu)
	}
	if cfg.Signup.OTPTTL != 10*time.Minute {
		t.Fatalf("unexpected signup OTP TTL default: %v", cfg.Signup.OTPTTL)
	}
	if cfg.Argon2.MemoryKiB != 64*1024 || cfg.Argon2.Iterations != 3 || cfg.Argon2.Parallelism != 2 {
		t.Fatalf("unexpected argon2 defaults: %+v", cfg.Argon2)
	}
	if cfg.OfflineLease.TTL != 168*time.Hour {
		t.Fatalf("unexpected offline lease TTL default: %v", cfg.OfflineLease.TTL)
	}
	if cfg.OfflineLease.PrivateKey != nil || cfg.OfflineLease.PublicKey != nil {
		t.Fatalf("expected no offline signing keys by default, got %+v", cfg.OfflineLease)
	}
}

func TestLoadOverrides(t *testing.T) {
	values := map[string]string{
		"JBM_ENV":                      Staging,
		"JBM_HTTP_ADDR":                "127.0.0.1:9090",
		"JBM_HTTP_READ_HEADER_TIMEOUT": "3s",
		"JBM_HTTP_READ_TIMEOUT":        "7s",
		"JBM_HTTP_WRITE_TIMEOUT":       "12s",
		"JBM_HTTP_IDLE_TIMEOUT":        "45s",
		"JBM_HTTP_SHUTDOWN_TIMEOUT":    "8s",
		"JBM_DATABASE_URL":             "postgres://localhost/custom",
		"JBM_DB_MAX_CONNS":             "20",
		"JBM_DB_MIN_CONNS":             "0",
		"JBM_DB_MAX_CONN_LIFETIME":     "1h",
		"JBM_DB_MAX_CONN_IDLE_TIME":    "10m",
		"JBM_DB_HEALTH_TIMEOUT":        "750ms",
		"JBM_LOG_LEVEL":                "WARN",
	}

	cfg, err := load(mapLookup(values))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if cfg.Env != Staging || cfg.HTTP.Addr != "127.0.0.1:9090" || cfg.LogLevel != "warn" {
		t.Fatalf("unexpected overrides: %+v", cfg)
	}
	if cfg.HTTP.ReadHeaderTimeout != 3*time.Second || cfg.HTTP.ShutdownTimeout != 8*time.Second {
		t.Fatalf("unexpected HTTP overrides: %+v", cfg.HTTP)
	}
	if cfg.Database.MaxConns != 20 || cfg.Database.MinConns != 0 || cfg.Database.HealthTimeout != 750*time.Millisecond {
		t.Fatalf("unexpected database overrides: %+v", cfg.Database)
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		message string
	}{
		{name: "environment", key: "JBM_ENV", value: "local", message: "JBM_ENV"},
		{name: "empty address", key: "JBM_HTTP_ADDR", value: " ", message: "JBM_HTTP_ADDR"},
		{name: "duration without unit", key: "JBM_HTTP_READ_TIMEOUT", value: "10", message: "positive duration"},
		{name: "zero duration", key: "JBM_DB_HEALTH_TIMEOUT", value: "0s", message: "positive duration"},
		{name: "negative connections", key: "JBM_DB_MIN_CONNS", value: "-1", message: "non-negative integer"},
		{name: "zero max connections", key: "JBM_DB_MAX_CONNS", value: "0", message: "positive integer"},
		{name: "invalid log level", key: "JBM_LOG_LEVEL", value: "trace", message: "JBM_LOG_LEVEL"},
		{name: "session ttl without unit", key: "JBM_SESSION_TTL", value: "720", message: "positive duration"},
		{name: "session cookie secure not a bool", key: "JBM_SESSION_COOKIE_SECURE", value: "yes", message: "must be true or false"},
		{name: "platform session ttl without unit", key: "JBM_PLATFORM_SESSION_TTL", value: "12", message: "positive duration"},
		{name: "signup otp ttl without unit", key: "JBM_SIGNUP_OTP_TTL", value: "10", message: "positive duration"},
		{name: "smtp max send attempts zero", key: "JBM_SMTP_MAX_SEND_ATTEMPTS", value: "0", message: "positive integer"},
		{name: "smtp max send attempts non-numeric", key: "JBM_SMTP_MAX_SEND_ATTEMPTS", value: "many", message: "positive integer"},
		{name: "smtp retry base delay without unit", key: "JBM_SMTP_RETRY_BASE_DELAY", value: "500", message: "positive duration"},
		{name: "zavu max send attempts zero", key: "JBM_ZAVU_MAX_SEND_ATTEMPTS", value: "0", message: "positive integer"},
		{name: "zavu retry base delay without unit", key: "JBM_ZAVU_RETRY_BASE_DELAY", value: "500", message: "positive duration"},
		{name: "zero argon2 memory", key: "JBM_ARGON2_MEMORY_KIB", value: "0", message: "positive integer"},
		{name: "non-numeric argon2 iterations", key: "JBM_ARGON2_ITERATIONS", value: "many", message: "positive integer"},
		{name: "argon2 parallelism too large", key: "JBM_ARGON2_PARALLELISM", value: "300", message: "no greater than 255"},
		{name: "offline lease ttl zero", key: "JBM_OFFLINE_LEASE_TTL", value: "0h", message: "positive duration"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{"JBM_DATABASE_URL": "postgres://localhost/justbarme", test.key: test.value}
			_, err := load(mapLookup(values))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("load() error = %v, want message containing %q", err, test.message)
			}
		})
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	_, err := load(mapLookup(nil))
	if err == nil || !strings.Contains(err.Error(), "JBM_DATABASE_URL is required") {
		t.Fatalf("load() error = %v", err)
	}
}

func TestLoadRejectsMinConnectionsAboveMax(t *testing.T) {
	_, err := load(mapLookup(map[string]string{
		"JBM_DATABASE_URL": "postgres://localhost/justbarme",
		"JBM_DB_MIN_CONNS": "11",
		"JBM_DB_MAX_CONNS": "10",
	}))
	if err == nil || !strings.Contains(err.Error(), "must not exceed") {
		t.Fatalf("load() error = %v", err)
	}
}

func TestLoadOfflineSigningKeys(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privB64 := base64.StdEncoding.EncodeToString(priv)
	pubB64 := base64.StdEncoding.EncodeToString(pub)

	t.Run("valid matching pair loads", func(t *testing.T) {
		cfg, err := load(mapLookup(map[string]string{
			"JBM_DATABASE_URL":                "postgres://localhost/justbarme",
			"JBM_OFFLINE_SIGNING_PRIVATE_KEY": privB64,
			"JBM_OFFLINE_SIGNING_PUBLIC_KEY":  pubB64,
		}))
		if err != nil {
			t.Fatalf("load() error = %v", err)
		}
		if !cfg.OfflineLease.PrivateKey.Equal(priv) || !cfg.OfflineLease.PublicKey.Equal(pub) {
			t.Fatal("expected decoded keys to match the configured keypair")
		}
	})

	t.Run("only private key set is rejected", func(t *testing.T) {
		_, err := load(mapLookup(map[string]string{
			"JBM_DATABASE_URL":                "postgres://localhost/justbarme",
			"JBM_OFFLINE_SIGNING_PRIVATE_KEY": privB64,
		}))
		if err == nil || !strings.Contains(err.Error(), "must both be set") {
			t.Fatalf("load() error = %v", err)
		}
	})

	t.Run("mismatched pair is rejected", func(t *testing.T) {
		_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
		_, err := load(mapLookup(map[string]string{
			"JBM_DATABASE_URL":                "postgres://localhost/justbarme",
			"JBM_OFFLINE_SIGNING_PRIVATE_KEY": base64.StdEncoding.EncodeToString(otherPriv),
			"JBM_OFFLINE_SIGNING_PUBLIC_KEY":  pubB64,
		}))
		if err == nil || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("load() error = %v", err)
		}
	})

	t.Run("malformed base64 is rejected", func(t *testing.T) {
		_, err := load(mapLookup(map[string]string{
			"JBM_DATABASE_URL":                "postgres://localhost/justbarme",
			"JBM_OFFLINE_SIGNING_PRIVATE_KEY": "not-base64!!",
			"JBM_OFFLINE_SIGNING_PUBLIC_KEY":  pubB64,
		}))
		if err == nil || !strings.Contains(err.Error(), "JBM_OFFLINE_SIGNING_PRIVATE_KEY") {
			t.Fatalf("load() error = %v", err)
		}
	})
}

func TestLoadProductionRequiresHardenedConfiguration(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	base := map[string]string{
		"JBM_DATABASE_URL": "postgres://localhost/justbarme",
		"JBM_ENV":          Production,
	}

	t.Run("missing offline keys, insecure cookie, and http base URL are all rejected", func(t *testing.T) {
		_, err := load(mapLookup(base))
		if err == nil {
			t.Fatal("expected production defaults to be rejected")
		}
		for _, want := range []string{"JBM_OFFLINE_SIGNING_PRIVATE_KEY", "JBM_PUBLIC_BASE_URL must be an https URL", "JBM_SMTP_USERNAME", "JBM_ZAVU_API_KEY", "JBM_STORAGE_ENDPOINT"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("load() error = %v, want it to mention %q", err, want)
			}
		}
	})

	t.Run("fully hardened production configuration passes", func(t *testing.T) {
		values := map[string]string{
			"JBM_DATABASE_URL":                "postgres://localhost/justbarme",
			"JBM_ENV":                         Production,
			"JBM_SESSION_COOKIE_SECURE":       "true",
			"JBM_OFFLINE_SIGNING_PRIVATE_KEY": base64.StdEncoding.EncodeToString(priv),
			"JBM_OFFLINE_SIGNING_PUBLIC_KEY":  base64.StdEncoding.EncodeToString(pub),
			"JBM_PUBLIC_BASE_URL":             "https://justbarme.app",
			"JBM_SMTP_USERNAME":               "signup@justbarme.app",
			"JBM_SMTP_PASSWORD":               "app-password",
			"JBM_SMTP_FROM":                   "justbarme <signup@justbarme.app>",
			"JBM_ZAVU_API_KEY":                "zv_live_test",
			"JBM_ZAVU_SENDER_ID":              "sender-123",
			"JBM_STORAGE_ENDPOINT":            "https://account123.r2.cloudflarestorage.com",
			"JBM_STORAGE_ACCESS_KEY_ID":       "storage-key-id",
			"JBM_STORAGE_SECRET_ACCESS_KEY":   "storage-secret",
			"JBM_STORAGE_BUCKET":              "justbarme-uploads",
			"JBM_STORAGE_PUBLIC_BASE_URL":     "https://uploads.justbarme.app",
		}
		if _, err := load(mapLookup(values)); err != nil {
			t.Fatalf("load() error = %v", err)
		}
	})
}

func BenchmarkLoad(b *testing.B) {
	lookup := mapLookup(map[string]string{
		"JBM_DATABASE_URL":          "postgres://localhost/justbarme",
		"JBM_HTTP_READ_TIMEOUT":     "10s",
		"JBM_DB_MAX_CONNS":          "10",
		"JBM_DB_MAX_CONN_IDLE_TIME": "5m",
	})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := load(lookup); err != nil {
			b.Fatal(err)
		}
	}
}

func mapLookup(values map[string]string) lookupEnv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
