// Package oauth implements "Sign in with Google" as a second way into the
// same account system internal/signup and internal/identity already run --
// there is no separate identity space here, unlike internal/platformadmin.
// This is another deliberate deviation from docs/ (which never mentions
// OAuth at all): see CLAUDE.md's OAuth section for the reasoning and the
// three account-resolution cases SignInWithGoogle handles.
package oauth

// GoogleClaims are the subset of a verified Google ID token's claims this
// package needs. Sub is Google's stable per-account identifier -- the
// value actually stored in user_identities, never Email, which a person
// can change on their Google account without Sub changing.
type GoogleClaims struct {
	Sub           string
	Email         string
	EmailVerified bool
	Name          string
}
