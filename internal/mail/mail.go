// Package mail provides transactional email senders (spec §25 verification/
// invite/reset). A log-only sender is used in dev when SMTP is unconfigured.
package mail

import (
	"fmt"
	"log/slog"
	"net/smtp"
)

// LogMailer logs emails instead of sending (dev/local).
type LogMailer struct{ Log *slog.Logger }

// Send logs the message.
func (m LogMailer) Send(to, subject, body string) error {
	m.Log.Info("email (dev)", "to", to, "subject", subject, "body", body)
	return nil
}

// SMTPMailer sends via SMTP.
type SMTPMailer struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

// Send delivers a plaintext email.
func (m SMTPMailer) Send(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", m.Host, m.Port)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s\r\n", m.From, to, subject, body)
	var auth smtp.Auth
	if m.User != "" {
		auth = smtp.PlainAuth("", m.User, m.Pass, m.Host)
	}
	return smtp.SendMail(addr, auth, m.From, []string{to}, []byte(msg))
}
