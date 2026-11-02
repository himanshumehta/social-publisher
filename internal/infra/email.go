package infra

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// EmailSender sends transactional emails via SMTP. If host is empty, emails
// are logged to stdout instead (dev mode).
type EmailSender struct {
	host     string
	port     string
	username string
	password string
	from     string
	useTLS   bool
}

func NewEmailSender(host, port, username, password, from string, useTLS bool) *EmailSender {
	return &EmailSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		useTLS:   useTLS,
	}
}

// Send dispatches an email. In dev mode (host == ""), it logs to slog instead.
func (e *EmailSender) Send(to, subject, htmlBody string) error {
	if e.host == "" {
		slog.Info("[DEV EMAIL]",
			"to", to,
			"subject", subject,
			"body_len", len(htmlBody),
		)
		return nil
	}

	addr := e.host + ":" + e.port

	headers := map[string]string{
		"From":         e.from,
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=\"utf-8\"",
	}

	var msg strings.Builder
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(htmlBody)

	auth := smtp.PlainAuth("", e.username, e.password, e.host)

	if e.useTLS {
		tlsConfig := &tls.Config{ServerName: e.host}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("tls dial: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, e.host)
		if err != nil {
			return fmt.Errorf("smtp client: %w", err)
		}
		defer client.Close()

		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
		if err := client.Mail(e.from); err != nil {
			return fmt.Errorf("smtp mail from: %w", err)
		}
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("smtp rcpt: %w", err)
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("smtp data: %w", err)
		}
		if _, err := w.Write([]byte(msg.String())); err != nil {
			return fmt.Errorf("smtp write: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("smtp close data: %w", err)
		}
		return client.Quit()
	}

	return smtp.SendMail(addr, auth, e.from, []string{to}, []byte(msg.String()))
}
