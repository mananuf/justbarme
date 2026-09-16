package email

import (
	"bytes"
	"fmt"
	"text/template"
)

// Template renders a Message's Subject and Text from one shared data
// value, so every transactional email -- signup OTP today, whatever kind
// comes next -- is built the same way instead of each caller hand-writing
// fmt.Sprintf calls. Subject and body are separate text/template
// instances: a subject must never pick up a stray newline a multi-line
// body template could introduce.
type Template struct {
	name    string
	subject *template.Template
	body    *template.Template
}

// NewTemplate parses subjectSrc and bodySrc once. Callers should do this at
// package-init time (see internal/signup's otpEmailTemplate) so a malformed
// template is a startup-time failure, never something a live user's
// request can hit.
func NewTemplate(name, subjectSrc, bodySrc string) (*Template, error) {
	subject, err := template.New(name + ".subject").Parse(subjectSrc)
	if err != nil {
		return nil, fmt.Errorf("parse %s subject template: %w", name, err)
	}
	body, err := template.New(name + ".body").Parse(bodySrc)
	if err != nil {
		return nil, fmt.Errorf("parse %s body template: %w", name, err)
	}
	return &Template{name: name, subject: subject, body: body}, nil
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
	return Message{To: to, Subject: subjectBuf.String(), Text: bodyBuf.String()}, nil
}
