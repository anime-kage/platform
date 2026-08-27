// Package mail sends transactional email — currently just password resets.
//
// It speaks SMTP from the standard library rather than one provider's HTTP
// API, because every provider worth using (Resend, Brevo, Mailgun, Postmark,
// even Gmail) accepts SMTP. That keeps the choice of provider an env-var
// decision instead of a code change, and adds no dependency.
//
// With no SMTP_HOST configured the sender logs the message instead of
// sending it. That is deliberate: password reset must be testable in dev
// without an email account, and a silent no-op would be much worse than a
// visible log line. It is never a fallback in production — see Configured.
package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	// From is the envelope sender, e.g. "Anime-Kage <no-reply@anime-kage.ro>".
	From string
}

type Sender struct {
	cfg Config
}

func New(cfg Config) *Sender { return &Sender{cfg: cfg} }

// Configured reports whether real sending is possible. Handlers use it to
// decide whether an unconfigured mailer is acceptable, not to skip sending.
func (s *Sender) Configured() bool { return s.cfg.Host != "" }

type Message struct {
	To      string
	Subject string
	// Text is the whole body. Deliberately plain text, no HTML alternative:
	// a reset mail is one sentence and one link, HTML would only make it
	// likelier to be filtered as spam and harder to read in a terminal client.
	Text string
}

func (s *Sender) Send(ctx context.Context, m Message) error {
	if !s.Configured() {
		// The link is the whole point of the mail, so log the body — this is
		// how you complete a reset in dev.
		slog.Warn("SMTP not configured — logging mail instead of sending",
			"to", m.To, "subject", m.Subject, "body", m.Text)
		return nil
	}

	raw := s.compose(m)
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))

	var auth smtp.Auth
	if s.cfg.Username != "" {
		auth = smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	}

	// net/smtp has no context support, so run it on a goroutine and let the
	// caller's deadline win the race. The send is not cancelled — nothing in
	// net/smtp can do that — but the request stops waiting on it.
	done := make(chan error, 1)
	go func() {
		done <- smtp.SendMail(addr, auth, s.senderAddress(), []string{m.To}, raw)
	}()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("send mail: %w", err)
		}
		return nil
	case <-ctx.Done():
		return errors.New("send mail: timed out")
	}
}

func (s *Sender) compose(m Message) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", s.cfg.From)
	fmt.Fprintf(&b, "To: %s\r\n", m.To)
	// Subjects carry Romanian diacritics, which are not ASCII — RFC 2047
	// encoding is what keeps "Resetare parolă" from arriving as mojibake.
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	// Bare newlines are legal in a body but SMTP wants CRLF line endings.
	b.WriteString(strings.ReplaceAll(m.Text, "\n", "\r\n"))
	return []byte(b.String())
}

// senderAddress strips a display name down to the bare address for the SMTP
// envelope: "Anime-Kage <no-reply@x.ro>" is a valid From header but not a
// valid MAIL FROM argument.
func (s *Sender) senderAddress() string {
	from := s.cfg.From
	if i := strings.LastIndex(from, "<"); i >= 0 {
		if j := strings.Index(from[i:], ">"); j > 0 {
			return from[i+1 : i+j]
		}
	}
	return strings.TrimSpace(from)
}
