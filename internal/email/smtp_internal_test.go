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
}
