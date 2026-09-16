// Command seed-platform-staff creates one platform staff account -- a
// support or superadmin operator who can oversee businesses across the
// whole service (internal/platformadmin). There is no signup endpoint for
// this tier, doubly so compared to cmd/seed for business users: run this
// against a trusted database connection only.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/mananuf/justbarme/internal/app"
	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/platformadmin"
	"github.com/mananuf/justbarme/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed-platform-staff:", err)
		os.Exit(1)
	}
}

func run() error {
	if err := app.LoadDotEnvIfPresent(); err != nil {
		return fmt.Errorf("load .env: %w", err)
	}

	email := flag.String("email", "", "email address for the new platform staff account (required)")
	password := flag.String("password", "", "password for the new account (required, at least 8 characters)")
	name := flag.String("name", "", "display name for the new account (required)")
	role := flag.String("role", platformadmin.RoleSupport, "role: support or superadmin")
	flag.Parse()

	if *email == "" || *password == "" || *name == "" {
		flag.Usage()
		return errors.New("-email, -password, and -name are required")
	}
	if len(*password) < 8 {
		return errors.New("-password must be at least 8 characters")
	}
	if *role != platformadmin.RoleSupport && *role != platformadmin.RoleSuperadmin {
		return fmt.Errorf("-role must be %q or %q", platformadmin.RoleSupport, platformadmin.RoleSuperadmin)
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

	svc := platformadmin.New(pool, cfg.Argon2)
	staff, err := svc.CreateStaff(ctx, *email, *name, *password, *role)
	if err != nil {
		return fmt.Errorf("create platform staff: %w", err)
	}
	fmt.Printf("created platform staff %s <%s> (id=%s, role=%s)\n", staff.DisplayName, staff.Email, staff.ID, staff.Role)
	return nil
}
