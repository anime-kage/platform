package auth

import (
	"strings"
	"testing"
	"time"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "correct horse battery" {
		t.Fatal("hash equals plaintext")
	}
	if !CheckPassword("correct horse battery", hash) {
		t.Error("correct password rejected")
	}
	if CheckPassword("wrong password!!", hash) {
		t.Error("wrong password accepted")
	}
}

func TestSignVerifyRoundtrip(t *testing.T) {
	m := NewManager("test-secret", time.Hour)
	token, err := m.Sign(42, "ana", "ana@example.com", "translator")
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	claims, err := m.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != 42 || claims.Username != "ana" ||
		claims.Email != "ana@example.com" || claims.Role != "translator" {
		t.Errorf("claims mismatch: %+v", claims)
	}
}

func TestVerifyRejections(t *testing.T) {
	m := NewManager("test-secret", time.Hour)
	good, _ := m.Sign(1, "u", "u@e.com", "user")

	t.Run("wrong secret", func(t *testing.T) {
		other := NewManager("different-secret", time.Hour)
		if _, err := other.Verify(good); err == nil {
			t.Error("token signed with another secret was accepted")
		}
	})
	t.Run("expired", func(t *testing.T) {
		past := NewManager("test-secret", -time.Hour)
		token, _ := past.Sign(1, "u", "u@e.com", "user")
		if _, err := m.Verify(token); err == nil {
			t.Error("expired token was accepted")
		}
	})
	t.Run("tampered payload", func(t *testing.T) {
		parts := strings.Split(good, ".")
		parts[1] = strings.Repeat("A", len(parts[1]))
		if _, err := m.Verify(strings.Join(parts, ".")); err == nil {
			t.Error("tampered token was accepted")
		}
	})
	t.Run("garbage", func(t *testing.T) {
		if _, err := m.Verify("not.a.jwt"); err == nil {
			t.Error("garbage token was accepted")
		}
	})
}

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header, want string
	}{
		{"Bearer abc.def.ghi", "abc.def.ghi"},
		{"Bearer ", ""},
		{"bearer abc", ""}, // scheme is case-sensitive, matching the old backend
		{"Basic abc", ""},
		{"", ""},
		{"abc.def.ghi", ""},
	}
	for _, tt := range tests {
		if got := BearerToken(tt.header); got != tt.want {
			t.Errorf("BearerToken(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestValidateRegister(t *testing.T) {
	tests := []struct {
		name, username, email, password string
		wantErr                         bool
	}{
		{"valid", "ana_maria1", "ana@example.com", "longpassword", false},
		{"username too short", "ab", "ana@example.com", "longpassword", true},
		{"username too long", strings.Repeat("a", 51), "ana@example.com", "longpassword", true},
		{"username bad chars", "ana maria", "ana@example.com", "longpassword", true},
		{"bad email", "anamaria", "not-an-email", "longpassword", true},
		{"password 7 chars", "anamaria", "ana@example.com", "sevench", true},
		{"password 8 chars ok", "anamaria", "ana@example.com", "eightchr", false},
		// counted in runes: seven letters is short however many bytes the
		// diacritics occupy (this one is 10 bytes)
		{"password 7 chars with diacritics", "anamaria", "ana@example.com", "parolăă", true},
		{"password 8 chars with diacritics ok", "anamaria", "ana@example.com", "parolăăă", false},
		// bcrypt's own ceiling — over this it used to validate and then 500
		{"password 72 bytes ok", "anamaria", "ana@example.com", strings.Repeat("p", 72), false},
		{"password too long", "anamaria", "ana@example.com", strings.Repeat("p", 73), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRegister(tt.username, tt.email, tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("got err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogin(t *testing.T) {
	if err := ValidateLogin("a@b.co", "x"); err != nil {
		t.Errorf("valid login rejected: %v", err)
	}
	if err := ValidateLogin("nope", "x"); err == nil {
		t.Error("bad email accepted")
	}
	if err := ValidateLogin("a@b.co", ""); err == nil {
		t.Error("empty password accepted")
	}
}
