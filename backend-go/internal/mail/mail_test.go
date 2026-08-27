package mail

import (
	"context"
	"strings"
	"testing"
)

func TestSenderAddress(t *testing.T) {
	tests := []struct{ from, want string }{
		{"Anime-Kage <no-reply@anime-kage.ro>", "no-reply@anime-kage.ro"},
		{"no-reply@anime-kage.ro", "no-reply@anime-kage.ro"},
		{"  spaced@example.com  ", "spaced@example.com"},
		{"Broken <no-close@example.com", "Broken <no-close@example.com"},
	}
	for _, tt := range tests {
		s := New(Config{From: tt.from})
		if got := s.senderAddress(); got != tt.want {
			t.Errorf("senderAddress(%q) = %q, want %q", tt.from, got, tt.want)
		}
	}
}

func TestComposeEncodesDiacriticsAndUsesCRLF(t *testing.T) {
	s := New(Config{From: "Anime-Kage <no-reply@anime-kage.ro>"})
	raw := string(s.compose(Message{
		To:      "ana@example.com",
		Subject: "Resetare parolă",
		Text:    "Prima linie\nA doua linie",
	}))

	// a raw non-ASCII byte in a header is what produces mojibake in clients
	if strings.Contains(strings.SplitN(raw, "\r\n\r\n", 2)[0], "parolă") {
		t.Error("subject was not RFC 2047 encoded")
	}
	if !strings.Contains(raw, "charset=utf-8") {
		t.Error("missing utf-8 content type")
	}
	body := strings.SplitN(raw, "\r\n\r\n", 2)[1]
	if strings.Contains(strings.ReplaceAll(body, "\r\n", ""), "\n") {
		t.Error("body contains a bare LF, SMTP wants CRLF")
	}
}

func TestUnconfiguredSenderDoesNotError(t *testing.T) {
	s := New(Config{})
	if s.Configured() {
		t.Fatal("empty config reported as configured")
	}
	// dev path: logs instead of sending, must not fail the request
	if err := s.Send(context.Background(), Message{To: "a@b.co", Subject: "x", Text: "y"}); err != nil {
		t.Errorf("unconfigured Send returned %v, want nil", err)
	}
}
