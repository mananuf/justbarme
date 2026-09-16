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

	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/httpapi"
	"github.com/mananuf/justbarme/internal/identity"
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

	handler := httpapi.NewHandler(httpapi.Dependencies{
		Logger:        logger,
		Database:      pool,
		HealthTimeout: cfg.Database.HealthTimeout,
		Version:       Version,
		Identity:      identitySvc,
		Catalogue:     catalogueSvc,
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
