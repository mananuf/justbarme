package store

import (
	"strings"
	"testing"
	"time"

	"github.com/mananuf/justbarme/internal/config"
)

func TestNewPoolConfig(t *testing.T) {
	cfg := config.Database{
		URL:             "postgres://user:password@localhost:5432/justbarme",
		MaxConns:        20,
		MinConns:        2,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 10 * time.Minute,
	}

	poolConfig, err := newPoolConfig(cfg)
	if err != nil {
		t.Fatalf("newPoolConfig() error = %v", err)
	}
	if poolConfig.MaxConns != cfg.MaxConns || poolConfig.MinConns != cfg.MinConns || poolConfig.MaxConnLifetime != cfg.MaxConnLifetime || poolConfig.MaxConnIdleTime != cfg.MaxConnIdleTime {
		t.Fatalf("pool config does not match input: %+v", poolConfig)
	}
	if poolConfig.ConnConfig.Database != "justbarme" {
		t.Fatalf("database = %q, want justbarme", poolConfig.ConnConfig.Database)
	}
}

func TestNewPoolConfigRejectsMalformedURL(t *testing.T) {
	_, err := newPoolConfig(config.Database{URL: "postgres://%"})
	if err == nil || !strings.Contains(err.Error(), "parse PostgreSQL configuration") {
		t.Fatalf("newPoolConfig() error = %v", err)
	}
}

func BenchmarkNewPoolConfig(b *testing.B) {
	cfg := config.Database{
		URL:             "postgres://user:password@localhost:5432/justbarme",
		MaxConns:        10,
		MinConns:        1,
		MaxConnLifetime: 30 * time.Minute,
		MaxConnIdleTime: 5 * time.Minute,
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := newPoolConfig(cfg); err != nil {
			b.Fatal(err)
		}
	}
}
