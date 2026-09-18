package email

import (
	"strings"
	"testing"
)

// Internal test (package email, not email_test) purely to exercise
// buildMessage's formatting -- SMTPProvider.Send itself needs a real
// network connection to an SMTP relay and is not unit-tested here.
func TestBuildMessageIncludesHeadersAndBody(t *testing.T) {
	got := string(buildMessage("justbarme <no-reply@justbarme.app>", Message{
		To: "owner@example.com", Subject: "Your code", Text: "Your code is 123456.",
	}))

	for _, want := range []string{
		"From: justbarme <no-reply@justbarme.app>",
		"To: owner@example.com",
		"Subject: Your code",
		"Your code is 123456.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected message to contain %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "multipart/alternative") {
		t.Fatalf("expected a plain Message (no HTML) to skip multipart entirely, got:\n%s", got)
	}
}

func TestBuildMessageSendsMultipartWhenHTMLIsPresent(t *testing.T) {
	got := string(buildMessage("justbarme <no-reply@justbarme.app>", Message{
		To: "owner@example.com", Subject: "Your code", Text: "Your code is 123456.",
		HTML: "<p>Your code is <strong>123456</strong>.</p>",
	}))

	for _, want := range []string{
		"Content-Type: multipart/alternative; boundary=\"" + mimeBoundary + "\"",
		"Content-Type: text/plain; charset=\"UTF-8\"",
		"Your code is 123456.",
		"Content-Type: text/html; charset=\"UTF-8\"",
		"<strong>123456</strong>",
		"--" + mimeBoundary + "--",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected multipart message to contain %q, got:\n%s", want, got)
		}
	}
	// The plain-text part must come before the HTML part (RFC 2046: a
	// client renders the LAST part it understands).
	if strings.Index(got, "Your code is 123456.") > strings.Index(got, "<strong>123456</strong>") {
		t.Fatalf("expected the plain-text part to precede the HTML part, got:\n%s", got)
	}
}
