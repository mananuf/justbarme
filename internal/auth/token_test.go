package auth

import "testing"

func TestGenerateTokenIsRandomAndURLSafe(t *testing.T) {
	a, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	b, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if a == b {
		t.Fatal("expected two generated tokens to differ")
	}
	for _, r := range a {
		if r == '+' || r == '/' || r == '=' {
			t.Fatalf("token %q contains non-URL-safe character %q", a, r)
		}
	}
}

func TestHashTokenIsDeterministicAndDistinct(t *testing.T) {
	if HashToken("same") != HashToken("same") {
		t.Fatal("expected HashToken to be deterministic")
	}
	if HashToken("a") == HashToken("b") {
		t.Fatal("expected different inputs to hash differently")
	}
}
