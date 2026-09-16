package httpapi

import (
	"net"
	"net/http"
	"strings"
)

// clientIP is used only to rate-limit the public signup endpoints
// alongside per-email limiting -- it is not an authoritative client
// identity and must never be used for authorization decisions. Trusts
// X-Forwarded-For's first entry, which is only meaningful behind a
// reverse proxy that sets it; falls back to the direct connection's
// address otherwise.
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first, _, _ := strings.Cut(forwarded, ",")
		return strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
