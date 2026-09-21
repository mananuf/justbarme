#!/bin/sh
# The one release procedure -- docs/PHASE_PILOT_RELEASE.md §6: "a
# documented, explicit step in the deploy pipeline (not implicit, not run
# automatically by the API container on startup): back up, `make
# migrate-up` against the target environment's Postgres container, then
# deploy/restart the API, then verify readiness. The same procedure
# whether CI runs it against staging automatically or a human runs it by
# hand for a production promotion." Run this ON the target VPS (staging or
# production) -- CI SSHes in and runs it; a production promotion is the
# same command run by hand. Never run migrations automatically from
# inside the API container or on its own startup.
set -eu

: "${COMPOSE_PROJECT_DIR:?set COMPOSE_PROJECT_DIR to the directory containing docker-compose.yml}"
: "${IMAGE_TAG:=latest}"

# `make migrate-up` reads JBM_DATABASE_URL from the VPS's own root .env
# (via the Makefile's `include .env`, same as every other make target --
# see CLAUDE.md's Commands section) -- it must point at the loopback port
# docker-compose.yml publishes (127.0.0.1:5432), NOT the container-internal
# `postgres:5432` host the api service's own environment override uses.
# Those are deliberately two different values for the same variable name
# in two different places -- see docs/DEPLOYMENT.md.

cd "$COMPOSE_PROJECT_DIR"
export IMAGE_TAG

echo "deploy-release: 1/4 backing up ${POSTGRES_DB:-justbarme}..."
sh ./scripts/backup.sh

echo "deploy-release: 2/4 running pending migrations..."
make migrate-up

echo "deploy-release: 3/4 pulling and restarting containers (image tag: ${IMAGE_TAG})..."
docker compose -f docker-compose.yml -f docker-compose.prod.yml pull
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d

echo "deploy-release: 4/4 verifying readiness..."
attempt=0
until curl -fsS http://127.0.0.1:8080/api/v1/health/ready > /dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 30 ]; then
        echo "deploy-release: FAILED -- /health/ready never became healthy after deploy" >&2
        exit 1
    fi
    sleep 2
done

echo "deploy-release: done -- ${IMAGE_TAG} is live and ready"
