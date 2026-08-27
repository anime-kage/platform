package handler_test

// Password reset. The happy path is one line; what earns tests is
// everything that must NOT happen — leaking who has an account, a link that
// works twice, an expired link, a password that survives the reset.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"animekage/backend/internal/repo"
)

// resetTokenFor reads back the hash we stored and, since the raw token never
// touches the database, we cannot recover it — so tests mint their own token
// and seed the hash directly, exactly as the handler would have.
func seedReset(t *testing.T, userID int, tokenHash string, expiresAt time.Time, used bool) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	var usedAt *time.Time
	if used {
		now := time.Now()
		usedAt = &now
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO password_resets (user_id, token_hash, expires_at, used_at)
		 VALUES ($1, $2, $3, $4)`, userID, tokenHash, expiresAt, usedAt); err != nil {
		t.Fatalf("seed reset: %v", err)
	}
}

func userIDByUsername(t *testing.T, username string) int {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	var id int
	if err := conn.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, username).Scan(&id); err != nil {
		t.Fatalf("look up user: %v", err)
	}
	return id
}

// liveResetCount counts unspent tokens — used to prove that asking twice
// leaves only the newest link alive.
func liveResetCount(t *testing.T, userID int) int {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	var n int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM password_resets WHERE user_id = $1 AND used_at IS NULL`,
		userID).Scan(&n); err != nil {
		t.Fatalf("count resets: %v", err)
	}
	return n
}

// An address with no account must be indistinguishable from one that has an
// account. On an invite-only site the member list is exactly what should not
// be enumerable, and a 404 here would hand it over one address at a time.
func TestForgotPasswordDoesNotLeakAccounts(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.11.0.1"}

	name := c.signup()

	statusReal, bodyReal := c.do("POST", "/api/auth/forgot-password", map[string]any{
		"email": name + "@test.local",
	})
	c.mustStatus(200, statusReal, bodyReal, "forgot for a real account")

	statusFake, bodyFake := c.do("POST", "/api/auth/forgot-password", map[string]any{
		"email": fmt.Sprintf("nobody_%d@test.local", os.Getpid()),
	})
	c.mustStatus(200, statusFake, bodyFake, "forgot for an unknown address")

	if bodyReal["message"] != bodyFake["message"] {
		t.Fatalf("responses differ and leak account existence:\n real: %v\n fake: %v",
			bodyReal["message"], bodyFake["message"])
	}
}

// Only the newest link stays live: clicking "trimite din nou" must not leave
// a trail of working links in the inbox.
func TestForgotPasswordSupersedesEarlierTokens(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.11.0.2"}

	name := c.signup()
	id := userIDByUsername(t, name)

	for i := 0; i < 3; i++ {
		status, body := c.do("POST", "/api/auth/forgot-password", map[string]any{
			"email": name + "@test.local",
		})
		c.mustStatus(200, status, body, "forgot request")
	}

	if n := liveResetCount(t, id); n != 1 {
		t.Fatalf("expected exactly 1 live token after 3 requests, got %d", n)
	}
}

func TestResetPasswordHappyPathAndSingleUse(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.11.0.3"}

	name := c.signup()
	id := userIDByUsername(t, name)

	token := fmt.Sprintf("tok_ok_%d", os.Getpid())
	seedReset(t, id, repo.HashResetToken(token), time.Now().Add(time.Hour), false)

	status, body := c.do("POST", "/api/auth/reset-password", map[string]any{
		"token": token, "password": "parolanouă123",
	})
	c.mustStatus(200, status, body, "reset with a good token")

	// the new password works
	status, body = c.do("POST", "/api/auth/login", map[string]any{
		"email": name + "@test.local", "password": "parolanouă123",
	})
	c.mustStatus(200, status, body, "login with the new password")

	// and the old one does not — the point of the whole exercise
	status, body = c.do("POST", "/api/auth/login", map[string]any{
		"email": name + "@test.local", "password": "longpassword123",
	})
	c.mustStatus(401, status, body, "login with the old password")

	// the link cannot be replayed
	status, body = c.do("POST", "/api/auth/reset-password", map[string]any{
		"token": token, "password": "altaparola123",
	})
	c.mustStatus(400, status, body, "second use of the same token")
	if msg, _ := body["error"].(string); !strings.Contains(msg, "folosit") {
		t.Fatalf("expected a used-token message, got %q", msg)
	}
}

func TestResetPasswordExpiredToken(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.11.0.4"}

	name := c.signup()
	id := userIDByUsername(t, name)

	token := fmt.Sprintf("tok_exp_%d", os.Getpid())
	seedReset(t, id, repo.HashResetToken(token), time.Now().Add(-time.Minute), false)

	status, body := c.do("POST", "/api/auth/reset-password", map[string]any{
		"token": token, "password": "parolanouă123",
	})
	c.mustStatus(400, status, body, "reset with an expired token")
	// "expired" and "invalid" are different problems — one means ask again,
	// the other means you followed the wrong link
	if msg, _ := body["error"].(string); !strings.Contains(msg, "expirat") {
		t.Fatalf("expected an expiry-specific message, got %q", msg)
	}

	// the password must be untouched
	status, body = c.do("POST", "/api/auth/login", map[string]any{
		"email": name + "@test.local", "password": "longpassword123",
	})
	c.mustStatus(200, status, body, "original password still works")
}

func TestResetPasswordUnknownToken(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.11.0.5"}

	status, body := c.do("POST", "/api/auth/reset-password", map[string]any{
		"token": "not-a-real-token", "password": "parolanouă123",
	})
	c.mustStatus(400, status, body, "reset with an unknown token")
}

// The reset form must enforce the same policy as registration — otherwise
// reset becomes the way around it.
func TestResetPasswordRejectsWeakPassword(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.11.0.6"}

	name := c.signup()
	id := userIDByUsername(t, name)

	token := fmt.Sprintf("tok_weak_%d", os.Getpid())
	seedReset(t, id, repo.HashResetToken(token), time.Now().Add(time.Hour), false)

	status, body := c.do("POST", "/api/auth/reset-password", map[string]any{
		"token": token, "password": "scurta",
	})
	c.mustStatus(400, status, body, "reset with a 6-character password")

	// a rejected attempt must not spend the token
	status, body = c.do("POST", "/api/auth/reset-password", map[string]any{
		"token": token, "password": "parolanouă123",
	})
	c.mustStatus(200, status, body, "token still usable after a rejected attempt")
}
