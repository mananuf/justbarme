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

func TestNewTemplateLeavesHTMLEmpty(t *testing.T) {
	tmpl := email.MustNewTemplate("text_only", "Subject", "Body {{.Name}}")
	msg, err := tmpl.Render("a@example.com", struct{ Name string }{Name: "Ada"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if msg.HTML != "" {
		t.Fatalf("expected a text-only Template to leave Message.HTML empty, got %q", msg.HTML)
	}
}

func TestHTMLTemplateRenderPopulatesHTML(t *testing.T) {
	tmpl, err := email.NewHTMLTemplate("welcome",
		"Welcome, {{.Name}}",
		"Hi {{.Name}}, your business is ready.",
		"<p>Hi <strong>{{.Name}}</strong>, your business is ready.</p>",
	)
	if err != nil {
		t.Fatalf("NewHTMLTemplate: %v", err)
	}

	msg, err := tmpl.Render("owner@example.com", struct{ Name string }{Name: "Ada"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(msg.Text, "Hi Ada,") {
		t.Fatalf("unexpected text body: %q", msg.Text)
	}
	if !strings.Contains(msg.HTML, "<strong>Ada</strong>") {
		t.Fatalf("unexpected html body: %q", msg.HTML)
	}
}

func TestMustNewHTMLTemplatePanicsOnInvalidHTMLSyntax(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected MustNewHTMLTemplate to panic when the html source is malformed")
		}
	}()
	email.MustNewHTMLTemplate("bad", "subject", "body", "{{.Unterminated")
}

func TestLayoutWrapsContentInSharedShell(t *testing.T) {
	html, err := email.Layout(email.LayoutData{
		Preheader: "Your code is on its way",
		Content:   "<h1>Hi Ada</h1>",
	})
	if err != nil {
		t.Fatalf("Layout: %v", err)
	}
	for _, want := range []string{"Your code is on its way", "<h1>Hi Ada</h1>", "JUSTBARME"} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected layout output to contain %q, got:\n%s", want, html)
		}
	}
}
