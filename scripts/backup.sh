#!/bin/sh
# Nightly logical backup -- docs/PHASE_PILOT_RELEASE.md §2: "nightly
# pg_dump from the Postgres container via a host cron job, pushed off-box
# to the same external object-storage account used for logos... never
# left only on the VPS's own disk." Run this on the VPS host (via cron),
# not inside any container -- it shells out to `docker compose exec` to
# reach the running postgres container, and to the `aws` CLI (works
# against Cloudflare R2 given --endpoint-url; R2 is fully S3-API-compatible)
# to push the result off-box. See docs/DEPLOYMENT.md for the cron entry
# and the required environment variables.
set -eu

: "${COMPOSE_PROJECT_DIR:?set COMPOSE_PROJECT_DIR to the directory containing docker-compose.yml}"
: "${POSTGRES_DB:?set POSTGRES_DB (same value docker-compose.yml's postgres service uses)}"
: "${POSTGRES_USER:?set POSTGRES_USER (same value docker-compose.yml's postgres service uses)}"
: "${BACKUP_S3_ENDPOINT:?set BACKUP_S3_ENDPOINT (the R2 account endpoint, same one JBM_STORAGE_ENDPOINT uses)}"
: "${BACKUP_S3_BUCKET:?set BACKUP_S3_BUCKET (a bucket/prefix separate from the logo-upload bucket)}"

cd "$COMPOSE_PROJECT_DIR"

timestamp=$(date -u +%Y%m%dT%H%M%SZ)
filename="justbarme-${timestamp}.sql.gz"
tmp_path="/tmp/${filename}"

echo "backup: dumping ${POSTGRES_DB} from the postgres container..."
docker compose exec -T postgres pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" | gzip > "$tmp_path"

echo "backup: uploading ${filename} to ${BACKUP_S3_BUCKET}..."
aws s3 cp "$tmp_path" "s3://${BACKUP_S3_BUCKET}/${filename}" --endpoint-url "$BACKUP_S3_ENDPOINT"

rm -f "$tmp_path"
echo "backup: done (${filename})"
