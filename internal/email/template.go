package email

import (
	"bytes"
	"fmt"
	"text/template"
)

// Template renders a Message's Subject, Text, and (optionally) HTML from
// one shared data value, so every transactional email -- signup OTP today,
// whatever kind comes next -- is built the same way instead of each caller
// hand-writing fmt.Sprintf calls. Subject, text body, and HTML body are
// separate text/template instances: a subject must never pick up a stray
// newline a multi-line body template could introduce.
type Template struct {
	name    string
	subject *template.Template
	body    *template.Template
	html    *template.Template // nil if this Template has no HTML variant
}

// NewTemplate parses subjectSrc and bodySrc once. Callers should do this at
// package-init time (see internal/signup's otpEmailTemplate) so a malformed
// template is a startup-time failure, never something a live user's
// request can hit. The resulting Message has no HTML part -- see
// NewHTMLTemplate for one that does.
func NewTemplate(name, subjectSrc, bodySrc string) (*Template, error) {
	return newTemplate(name, subjectSrc, bodySrc, "")
}

// MustNewTemplate is NewTemplate for package-level var initialization: a
// template that fails to parse is a programmer error and should panic at
// startup, not be handled per-call.
func MustNewTemplate(name, subjectSrc, bodySrc string) *Template {
	t, err := NewTemplate(name, subjectSrc, bodySrc)
	if err != nil {
		panic(err)
	}
	return t
}

// NewHTMLTemplate is NewTemplate plus an HTML body variant. htmlSrc is
// typically built with email.Layout (see layout.go) so every transactional
// email shares the same header/footer chrome regardless of which feature
// package sent it.
func NewHTMLTemplate(name, subjectSrc, textSrc, htmlSrc string) (*Template, error) {
	return newTemplate(name, subjectSrc, textSrc, htmlSrc)
}

// MustNewHTMLTemplate is NewHTMLTemplate for package-level var
// initialization -- see MustNewTemplate.
func MustNewHTMLTemplate(name, subjectSrc, textSrc, htmlSrc string) *Template {
	t, err := NewHTMLTemplate(name, subjectSrc, textSrc, htmlSrc)
	if err != nil {
		panic(err)
	}
	return t
}

func newTemplate(name, subjectSrc, textSrc, htmlSrc string) (*Template, error) {
	subject, err := template.New(name + ".subject").Parse(subjectSrc)
	if err != nil {
		return nil, fmt.Errorf("parse %s subject template: %w", name, err)
	}
	body, err := template.New(name + ".body").Parse(textSrc)
	if err != nil {
		return nil, fmt.Errorf("parse %s body template: %w", name, err)
	}
	t := &Template{name: name, subject: subject, body: body}
	if htmlSrc != "" {
		html, err := template.New(name + ".html").Parse(htmlSrc)
		if err != nil {
			return nil, fmt.Errorf("parse %s html template: %w", name, err)
		}
		t.html = html
	}
	return t, nil
}

// Render executes the template against data -- typically a small,
// caller-defined struct (e.g. a signup OTP code and its expiry) -- and
// returns a ready-to-send Message.
func (t *Template) Render(to string, data any) (Message, error) {
	var subjectBuf, bodyBuf bytes.Buffer
	if err := t.subject.Execute(&subjectBuf, data); err != nil {
		return Message{}, fmt.Errorf("render %s subject: %w", t.name, err)
	}
	if err := t.body.Execute(&bodyBuf, data); err != nil {
		return Message{}, fmt.Errorf("render %s body: %w", t.name, err)
	}
	msg := Message{To: to, Subject: subjectBuf.String(), Text: bodyBuf.String()}
	if t.html != nil {
		var htmlBuf bytes.Buffer
		if err := t.html.Execute(&htmlBuf, data); err != nil {
			return Message{}, fmt.Errorf("render %s html: %w", t.name, err)
		}
		msg.HTML = htmlBuf.String()
	}
	return msg, nil
}
