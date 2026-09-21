#!/bin/sh
# Runs automatically before nginx starts (see Dockerfile). Generates a
# throwaway self-signed certificate at the exact path nginx.conf.template
# expects a real Let's Encrypt certificate at, ONLY if nothing is there
# yet -- so a fresh `docker compose up` (before certbot has ever issued a
# real certificate) starts nginx successfully instead of crash-looping on
# a missing file, and so this never overwrites a real certificate once
# one exists. Browsers will show a certificate warning until the real
# one-time issuance step in docs/DEPLOYMENT.md replaces this.
set -eu

DOMAIN="${DOMAIN:-localhost}"
CERT_DIR="/etc/letsencrypt/live/${DOMAIN}"

if [ -f "${CERT_DIR}/fullchain.pem" ] && [ -f "${CERT_DIR}/privkey.pem" ]; then
    exit 0
fi

echo "ensure-self-signed-cert: no certificate for ${DOMAIN} yet, generating a temporary self-signed one"
mkdir -p "${CERT_DIR}"
openssl req -x509 -nodes -days 1 -newkey rsa:2048 \
    -keyout "${CERT_DIR}/privkey.pem" \
    -out "${CERT_DIR}/fullchain.pem" \
    -subj "/CN=${DOMAIN}"
