// Command seed-catalogue idempotently upserts the platform catalogue
// templates (internal/catalogue.NigerianBarCatalogue) that every business
// picks from during onboarding's "choose what you sell" step. Safe to run
// repeatedly, including after editing the seed list -- it upserts by
// template/variant name rather than inserting duplicates.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mananuf/justbarme/internal/app"
	"github.com/mananuf/justbarme/internal/catalogue"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed-catalogue:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := app.LoadDotEnvIfPresent(); err != nil {
		return fmt.Errorf("load .env: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	ctx := context.Background()
	pool, err := store.Open(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	svc := catalogue.New(pool)
	if err := svc.SeedTemplates(ctx, catalogue.NigerianBarCatalogue); err != nil {
		return fmt.Errorf("seed catalogue templates: %w", err)
	}
	fmt.Printf("seeded %d catalogue templates\n", len(catalogue.NigerianBarCatalogue))
	return nil
}
