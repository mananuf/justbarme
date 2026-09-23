# Multi-stage build for the Go API -- docs/PHASE_PILOT_RELEASE.md §2/§6
# ("§16.3's 'multi-stage Docker build' is followed as written"). The final
# image carries only the compiled binary and CA certificates, never the Go
# toolchain or source tree.

FROM golang:1.26.6-alpine AS build
WORKDIR /src

# Dependencies first, so an unrelated source change doesn't invalidate
# this layer's cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.20
# ca-certificates: outbound TLS (SMTP/Zavu/object storage/Google JWKS all
# need it). tzdata: businesses.timezone-aware queries assume real IANA
# zone data is available, which alpine doesn't ship by default.
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/api /usr/local/bin/api

# Runs as jbm_app-equivalent least privilege at the OS level too -- a
# fixed non-root UID, not root, per docs/ARCHITECTURE.md §17's general
# hardening posture.
RUN adduser -D -u 10001 jbm
USER jbm

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
