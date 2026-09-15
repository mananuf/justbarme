package auth

import (
	"testing"
	"time"
)

func TestLimiterAllowsUpToLimit(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("key") {
			t.Fatalf("expected request %d to be allowed", i+1)
		}
	}
	if l.Allow("key") {
		t.Fatal("expected 4th request within window to be denied")
	}
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	l := NewLimiter(1, time.Minute)
	if !l.Allow("a") {
		t.Fatal("expected first request for key a to be allowed")
	}
	if !l.Allow("b") {
		t.Fatal("expected first request for key b to be allowed, independent of key a")
	}
}

func TestLimiterResetsAfterWindow(t *testing.T) {
	l := NewLimiter(1, 20*time.Millisecond)
	if !l.Allow("key") {
		t.Fatal("expected first request to be allowed")
	}
	if l.Allow("key") {
		t.Fatal("expected second immediate request to be denied")
	}
	time.Sleep(30 * time.Millisecond)
	if !l.Allow("key") {
		t.Fatal("expected request after window to be allowed again")
	}
}
