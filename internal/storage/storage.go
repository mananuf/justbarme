// Package storage is the object-storage boundary docs/ARCHITECTURE.md §15
// calls for ("store logos in S3-compatible object storage") but never
// built -- see docs/PHASE_PILOT_RELEASE.md §3. Kept behind an interface
// per docs/IMPLEMENTATION_PLAN.md's "keep provider behind an interface",
// the same shape internal/email and internal/whatsapp already use.
package storage

import "context"

// Provider stores an opaque object and reports the URL it can be served
// from. Implementations must not log object bytes.
type Provider interface {
	// Put uploads data under key with the given content type, returning
	// the URL it can be fetched from afterward.
	Put(ctx context.Context, key string, contentType string, data []byte) (url string, err error)
	// URL deterministically builds the servable URL for a key that was
	// already uploaded, with no I/O -- callers that persist only the key
	// (e.g. businesses.logo_object_key) use this to reconstruct a URL on
	// read, rather than storing the URL itself, so a future change of
	// public base URL/CDN domain needs no data migration.
	URL(key string) string
	// Delete removes the object at key. Deleting a key that doesn't exist
	// is not an error -- callers never need to check existence first.
	Delete(ctx context.Context, key string) error
}
