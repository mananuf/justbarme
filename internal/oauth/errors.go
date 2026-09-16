package oauth

import "errors"

// ErrGoogleSignInFailed covers every way SignInWithGoogle can fail -- an
// unverifiable, malformed, expired, or wrong-audience token, or a token
// whose email Google itself has not verified. Deliberately one bucket:
// the same "one generic response, do not reveal which check failed" shape
// docs/API_CONTRACT.md §8 requires of password login, applied here even
// though this isn't literally that endpoint.
var ErrGoogleSignInFailed = errors.New("google sign-in failed")
