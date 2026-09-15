package auth

import "testing"

func testParams() Argon2Params {
	// Deliberately tiny cost so the test suite stays fast; production values
	// come from JBM_ARGON2_* configuration.
	return Argon2Params{MemoryKiB: 8 * 1024, Iterations: 1, Parallelism: 1}
}

func TestHashAndVerifyPassword(t *testing.T) {
	encoded, err := HashPassword("correct horse battery staple", testParams())
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	ok, err := VerifyPassword(encoded, "correct horse battery staple")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("expected correct password to verify")
	}

	ok, err = VerifyPassword(encoded, "wrong password")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Fatal("expected incorrect password to fail verification")
	}
}

func TestHashPasswordSaltsDiffer(t *testing.T) {
	a, err := HashPassword("same password", testParams())
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	b, err := HashPassword("same password", testParams())
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if a == b {
		t.Fatal("expected two hashes of the same password to differ (distinct salts)")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	for _, encoded := range []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=65536,t=3,p=2$short",
		"$bcrypt$v=19$m=65536,t=3,p=2$c2FsdA$aGFzaA",
	} {
		if _, err := VerifyPassword(encoded, "anything"); err == nil {
			t.Errorf("expected error for malformed hash %q", encoded)
		}
	}
}

func TestNeedsRehash(t *testing.T) {
	params := testParams()
	encoded, err := HashPassword("a password", params)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	if NeedsRehash(encoded, params) {
		t.Fatal("expected hash with matching params to not need rehash")
	}

	other := params
	other.Iterations = params.Iterations + 1
	if !NeedsRehash(encoded, other) {
		t.Fatal("expected hash with different params to need rehash")
	}
}
