package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/mananuf/justbarme/internal/config"
)

func TestNewServerAppliesConfiguration(t *testing.T) {
	cfg := config.HTTP{
		Addr:              "127.0.0.1:9000",
		ReadHeaderTimeout: time.Second,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      3 * time.Second,
		IdleTimeout:       4 * time.Second,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := NewServer(cfg, http.NotFoundHandler(), logger)

	got := server.httpServer
	if got.Addr != cfg.Addr || got.ReadHeaderTimeout != cfg.ReadHeaderTimeout || got.ReadTimeout != cfg.ReadTimeout || got.WriteTimeout != cfg.WriteTimeout || got.IdleTimeout != cfg.IdleTimeout {
		t.Fatalf("HTTP server does not match config: %+v", got)
	}
	if got.Handler == nil || got.ErrorLog == nil {
		t.Fatal("HTTP server dependencies are missing")
	}
}

func BenchmarkNewServer(b *testing.B) {
	cfg := config.HTTP{Addr: ":8080", ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: time.Minute}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := http.NotFoundHandler()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewServer(cfg, handler, logger)
	}
}
