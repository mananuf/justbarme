package email_test

import (
	"strings"
	"testing"

	"github.com/mananuf/justbarme/internal/email"
)

func TestTemplateRenderSubstitutesData(t *testing.T) {
	tmpl, err := email.NewTemplate("welcome",
		"Welcome to justbarme, {{.Name}}",
		"Hi {{.Name}},\n\nYour business \"{{.Business}}\" is ready to go.",
	)
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}

	msg, err := tmpl.Render("owner@example.com", struct {
		Name     string
		Business string
	}{Name: "Ada", Business: "The Place"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if msg.To != "owner@example.com" {
		t.Fatalf("expected To to be passed through unchanged, got %q", msg.To)
	}
	if msg.Subject != "Welcome to justbarme, Ada" {
		t.Fatalf("unexpected subject: %q", msg.Subject)
	}
	if !strings.Contains(msg.Text, "Hi Ada,") || !strings.Contains(msg.Text, `"The Place"`) {
		t.Fatalf("unexpected body: %q", msg.Text)
	}
}

func TestTemplateRenderErrorsOnUnknownField(t *testing.T) {
	tmpl, err := email.NewTemplate("broken", "{{.DoesNotExist}}", "body")
	if err != nil {
		t.Fatalf("NewTemplate: %v", err)
	}

	if _, err := tmpl.Render("a@example.com", struct{ Name string }{Name: "Ada"}); err == nil {
		t.Fatal("expected Render to fail when the data has no matching field, got nil")
	}
}

func TestNewTemplateRejectsInvalidSyntax(t *testing.T) {
	if _, err := email.NewTemplate("bad", "{{.Unterminated", "body"); err == nil {
		t.Fatal("expected NewTemplate to reject malformed subject template syntax, got nil")
	}
	if _, err := email.NewTemplate("bad", "subject", "{{.Unterminated"); err == nil {
		t.Fatal("expected NewTemplate to reject malformed body template syntax, got nil")
	}
}

func TestMustNewTemplatePanicsOnInvalidSyntax(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected MustNewTemplate to panic on invalid template syntax")
		}
	}()
	email.MustNewTemplate("bad", "{{.Unterminated", "body")
}
