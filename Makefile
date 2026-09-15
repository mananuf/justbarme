.PHONY: run test test-race vet fmt fmt-check benchmark check tidy \
	migrate-up migrate-down migrate-down-all migrate-create seed sqlc

# Explicit package roots, not ./..., because web/node_modules can contain
# vendored .go files (e.g. from JS packages with a reference Go port) with
# no go.mod of their own — under this module's go.mod, ./... would otherwise
# pull them into build/vet/test.
GO_PACKAGES := ./cmd/... ./internal/...

run:
	go run ./cmd/api

test:
	go test $(GO_PACKAGES)

test-race:
	go test -race $(GO_PACKAGES)

vet:
	go vet $(GO_PACKAGES)

# Same reasoning as GO_PACKAGES above: excludes .cache/ (local module cache)
# and any node_modules/ (JS dependencies that may vendor stray .go files).
FIND_GO_FILES := find . -name '*.go' -type f -not -path './.cache/*' -not -path '*/node_modules/*'

fmt:
	gofmt -w $$($(FIND_GO_FILES))

fmt-check:
	@test -z "$$(gofmt -l $$($(FIND_GO_FILES)))" || \
		(echo "Go files require formatting:"; gofmt -l $$($(FIND_GO_FILES)); exit 1)

benchmark:
	go test -run '^$$' -bench . -benchmem $(GO_PACKAGES)

check: fmt-check vet test

tidy:
	go mod tidy

# Migrations require the `migrate` CLI (https://github.com/golang-migrate/migrate)
# and JBM_DATABASE_URL exported in your shell, e.g. `set -a && source .env && set +a`.
migrate-up:
	migrate -path migrations -database "$${JBM_DATABASE_URL}" up

# Rolls back the single most recent migration. Use migrate-down-all to tear
# everything down (drops jbm_app and every table — local/dev use only).
migrate-down:
	migrate -path migrations -database "$${JBM_DATABASE_URL}" down 1

migrate-down-all:
	migrate -path migrations -database "$${JBM_DATABASE_URL}" down -all

# Usage: make migrate-create name=add_something
migrate-create:
	migrate create -ext sql -dir migrations -seq $(name)

# Regenerates internal/store/sqlc from migrations/ and internal/store/queries/.
# Requires the sqlc CLI (https://sqlc.dev).
sqlc:
	sqlc generate

# Bootstraps a user (and optionally a business they own) — see cmd/seed.
# Usage: make seed ARGS="-email=owner@example.com -password=... -name='Ada Obi' -business='The Place'"
seed:
	go run ./cmd/seed $(ARGS)
