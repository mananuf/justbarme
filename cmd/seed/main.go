// Command seed creates one user, optionally with a business it owns. It
// exists because Phase 2 does not yet implement invitations (see
// docs/IMPLEMENTATION_PLAN.md Phase 2 vs. Phase 3+): there is currently no
// HTTP endpoint that can create the very first account for a new
// deployment, or a second user to test membership/tenancy behavior against.
// This is an operator tool, not a public API surface — run it against a
// trusted database connection only.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/mananuf/justbarme/internal/config"
	"github.com/mananuf/justbarme/internal/identity"
	"github.com/mananuf/justbarme/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run() error {
	email := flag.String("email", "", "email address for the new user (required)")
	password := flag.String("password", "", "password for the new user (required, at least 8 characters)")
	name := flag.String("name", "", "display name for the new user (required)")
	business := flag.String("business", "", "optional business name; if set, the user is created as its Owner")
	flag.Parse()

	if *email == "" || *password == "" || *name == "" {
		flag.Usage()
		return errors.New("-email, -password, and -name are required")
	}
	if len(*password) < 8 {
		return errors.New("-password must be at least 8 characters")
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

	svc := identity.New(pool, cfg.Argon2)

	user, err := svc.CreateUser(ctx, *email, "", *name, *password)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	fmt.Printf("created user %s <%s> (id=%s)\n", user.DisplayName, user.Email, user.ID)

	if *business == "" {
		return nil
	}

	biz, membership, location, err := svc.CreateBusinessWithOwner(ctx, user.ID, *business)
	if err != nil {
		return fmt.Errorf("create business: %w", err)
	}
	fmt.Printf("created business %q (id=%s) with %s as %s, default location id=%s\n",
		biz.Name, biz.ID, user.Email, membership.Role, location.ID)
	return nil
}
