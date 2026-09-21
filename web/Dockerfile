# Multi-stage build: compile the Vite PWA, then serve the static output
# plus reverse-proxy /api/* through nginx -- docs/PHASE_PILOT_RELEASE.md
# §2 ("The frontend needs no separate host anywhere -- the production
# build is fully static assets, served by the same nginx container").
#
# VITE_OFFLINE_SIGNING_PUBLIC_KEY and VITE_GOOGLE_OAUTH_CLIENT_ID are
# baked in at build time (Vite inlines import.meta.env.VITE_* at build,
# not at container start) -- pass them as --build-arg, sourced from the
# same values the API container's JBM_OFFLINE_SIGNING_PUBLIC_KEY /
# JBM_GOOGLE_OAUTH_CLIENT_ID use. VITE_API_PROXY_TARGET is deliberately
# NOT here -- it only matters to the Vite *dev* server (see web/.env.example);
# the production build calls relative /api/* paths, which this nginx
# config proxies to the api service on the compose network.

FROM node:22-alpine AS build
WORKDIR /src
# .npmrc (legacy-peer-deps=true) must be present before `npm ci` runs --
# without it, npm resolves peer dependencies strictly and rejects this
# lockfile as out of sync, even though it's the exact lockfile `npm ci`
# would accept with .npmrc's setting in effect. Caught by trying `npm ci`
# in a real container during this phase's own Docker-build verification;
# `npm install`/`npm run build` on a host that already has .npmrc in its
# working directory never surfaces this.
COPY package.json package-lock.json .npmrc ./
RUN npm ci
COPY . .

ARG VITE_OFFLINE_SIGNING_PUBLIC_KEY
ARG VITE_GOOGLE_OAUTH_CLIENT_ID
ENV VITE_OFFLINE_SIGNING_PUBLIC_KEY=$VITE_OFFLINE_SIGNING_PUBLIC_KEY
ENV VITE_GOOGLE_OAUTH_CLIENT_ID=$VITE_GOOGLE_OAUTH_CLIENT_ID
RUN npm run build

FROM nginx:1.27-alpine
# openssl: only for ensure-self-signed-cert.sh's one-time placeholder
# certificate below -- never used for anything else.
RUN apk add --no-cache openssl
# nginx's own docker-entrypoint runs envsubst over every *.template file
# here into /etc/nginx/conf.d/ at container start, substituting ${DOMAIN}
# from the environment -- see docker-compose.yml's "environment: DOMAIN=...".
# This is the image's own supported mechanism, not custom scripting.
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
# Every executable script under /docker-entrypoint.d/ runs automatically,
# in filename order, before nginx starts -- also the image's own
# supported mechanism. This one exists so a fresh `docker compose up`
# never crash-loops nginx waiting on a certificate that doesn't exist yet
# (nginx's config parser fails hard if ssl_certificate points at a
# missing file) -- see docs/DEPLOYMENT.md for the real one-time
# certificate-issuance step this placeholder gets replaced by.
COPY ensure-self-signed-cert.sh /docker-entrypoint.d/40-ensure-self-signed-cert.sh
RUN chmod +x /docker-entrypoint.d/40-ensure-self-signed-cert.sh
COPY --from=build /src/dist /usr/share/nginx/html

EXPOSE 80 443
