package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"time"
)

// SMTPConfig holds the credentials for an SMTP relay. The pilot uses Gmail
// (smtp.gmail.com:587 with an app password, not the account password
// itself -- Gmail rejects plain account passwords for SMTP), but nothing
// here is Gmail-specific.
type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// dialTimeout bounds how long a Send can hang against an unreachable or
// slow relay -- net/smtp has no native context support, so this is the
// only cancellation this provider gets.
const dialTimeout = 10 * time.Second

type SMTPProvider struct {
	cfg SMTPConfig
}

func NewSMTPProvider(cfg SMTPConfig) *SMTPProvider {
	return &SMTPProvider{cfg: cfg}
}

func (p *SMTPProvider) Send(_ context.Context, msg Message) error {
	addr := net.JoinHostPort(p.cfg.Host, p.cfg.Port)
	conn, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return fmt.Errorf("dial smtp server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, p.cfg.Host)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: p.cfg.Host}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}

	auth := smtp.PlainAuth("", p.cfg.Username, p.cfg.Password, p.cfg.Host)
	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	if err := client.Mail(p.cfg.From); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("smtp rcpt to: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(buildMessage(p.cfg.From, msg)); err != nil {
		return fmt.Errorf("write smtp body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close smtp data: %w", err)
	}
	return client.Quit()
}

func buildMessage(from string, msg Message) []byte {
	return fmt.Appendf(nil,
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=\"UTF-8\"\r\n\r\n%s",
		from, msg.To, msg.Subject, msg.Text,
	)
}
