package handler_test

// Invite-only registration. The interesting behaviour is not that
// a good code works — it is what happens around the edges: a code spent
// twice, a signup that fails after the code was checked, a gate that is off.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"animekage/backend/internal/config"
)

// seedInvite inserts a code straight into the database, standing in for the
// Discord bot (which we cannot drive from a test).
func seedInvite(t *testing.T, code string, expiresAt *time.Time) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx,
		`INSERT INTO invites (code, discord_user_id, discord_username, expires_at)
		 VALUES ($1, '900000000000000001', 'qa_inviter', $2)`, code, expiresAt); err != nil {
		t.Fatalf("seed invite: %v", err)
	}
}

func inviteUsedBy(t *testing.T, code string) (used bool, userID *int) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	if err := conn.QueryRow(ctx,
		`SELECT used_at IS NOT NULL, used_by_user_id FROM invites WHERE code = $1`,
		code).Scan(&used, &userID); err != nil {
		t.Fatalf("read invite: %v", err)
	}
	return used, userID
}

func inviteOnly(cfg *config.Config) { cfg.InviteOnly = true }

func TestRegisterInviteOnly(t *testing.T) {
	srv, _ := newTestServerCfg(t, inviteOnly)
	c := &client{t: t, base: srv.URL, ip: "10.9.0.1"}

	name := fmt.Sprintf("inv_ok_%d", os.Getpid())
	seedInvite(t, "KAGE-AAAA-11", nil)

	// no code at all
	status, body := c.do("POST", "/api/auth/register", map[string]any{
		"username": name, "email": name + "@test.local", "password": "longpassword123",
	})
	c.mustStatus(400, status, body, "register without code")

	// a code that was never minted
	status, body = c.do("POST", "/api/auth/register", map[string]any{
		"username": name, "email": name + "@test.local",
		"password": "longpassword123", "inviteCode": "KAGE-ZZZZ-99",
	})
	c.mustStatus(400, status, body, "register with unknown code")

	// the real one, typed in lower case — codes are stored upper-case and the
	// handler normalises, so this must still work
	status, body = c.do("POST", "/api/auth/register", map[string]any{
		"username": name, "email": name + "@test.local",
		"password": "longpassword123", "inviteCode": "kage-aaaa-11",
	})
	c.mustStatus(201, status, body, "register with valid code")

	used, userID := inviteUsedBy(t, "KAGE-AAAA-11")
	if !used || userID == nil {
		t.Fatalf("invite not marked used: used=%v userID=%v", used, userID)
	}

	// second use of the same code must fail
	name2 := name + "_b"
	status, body = c.do("POST", "/api/auth/register", map[string]any{
		"username": name2, "email": name2 + "@test.local",
		"password": "longpassword123", "inviteCode": "KAGE-AAAA-11",
	})
	c.mustStatus(400, status, body, "reuse of a spent code")
}

func TestRegisterInviteExpired(t *testing.T) {
	srv, _ := newTestServerCfg(t, inviteOnly)
	c := &client{t: t, base: srv.URL, ip: "10.9.0.2"}

	past := time.Now().Add(-time.Hour)
	seedInvite(t, "KAGE-BBBB-22", &past)

	name := fmt.Sprintf("inv_exp_%d", os.Getpid())
	status, body := c.do("POST", "/api/auth/register", map[string]any{
		"username": name, "email": name + "@test.local",
		"password": "longpassword123", "inviteCode": "KAGE-BBBB-22",
	})
	c.mustStatus(400, status, body, "register with expired code")
	// the user must be told it expired, not just "invalid" — otherwise they
	// retype a good code over and over
	if msg, _ := body["error"].(string); !strings.Contains(msg, "expirat") {
		t.Fatalf("expected an expiry-specific message, got %q", msg)
	}
}

// The ordering inside CreateUserWithInvite exists for this case: the account
// insert happens first and the claim second, both in one transaction. A
// signup that fails on a duplicate username must roll back the claim, or the
// member loses an invite to a typo.
func TestRegisterInviteNotBurnedByFailedSignup(t *testing.T) {
	srv, _ := newTestServerCfg(t, inviteOnly)
	c := &client{t: t, base: srv.URL, ip: "10.9.0.3"}

	seedInvite(t, "KAGE-CCCC-33", nil)
	seedInvite(t, "KAGE-DDDD-44", nil)

	taken := fmt.Sprintf("inv_taken_%d", os.Getpid())
	status, body := c.do("POST", "/api/auth/register", map[string]any{
		"username": taken, "email": taken + "@test.local",
		"password": "longpassword123", "inviteCode": "KAGE-CCCC-33",
	})
	c.mustStatus(201, status, body, "first signup")

	// same username, different code — the signup fails on the unique index
	status, body = c.do("POST", "/api/auth/register", map[string]any{
		"username": taken, "email": taken + "_other@test.local",
		"password": "longpassword123", "inviteCode": "KAGE-DDDD-44",
	})
	c.mustStatus(400, status, body, "duplicate username")

	if used, _ := inviteUsedBy(t, "KAGE-DDDD-44"); used {
		t.Fatal("invite was spent by a signup that failed — the transaction did not roll back")
	}
}

// With the gate off (the default) registration must behave exactly as before,
// ignoring any code that happens to be sent.
func TestRegisterOpenWhenGateOff(t *testing.T) {
	srv, _ := newTestServer(t)
	c := &client{t: t, base: srv.URL, ip: "10.9.0.4"}

	name := fmt.Sprintf("inv_off_%d", os.Getpid())
	status, body := c.do("POST", "/api/auth/register", map[string]any{
		"username": name, "email": name + "@test.local", "password": "longpassword123",
	})
	c.mustStatus(201, status, body, "register with gate off")
}

func TestPublicConfigReportsGate(t *testing.T) {
	srv, _ := newTestServerCfg(t, inviteOnly)
	c := &client{t: t, base: srv.URL, ip: "10.9.0.5"}

	status, body := c.do("GET", "/api/config", nil)
	c.mustStatus(200, status, body, "public config")
	data, _ := body["data"].(map[string]any)
	if data == nil || data["inviteOnly"] != true {
		t.Fatalf("expected inviteOnly=true, got %v", body)
	}
}

// Regression for the 0022 CHECK: `used_by_user_id` is ON DELETE SET NULL, so
// deleting a member who had joined by invite nulled that column while
// `used_at` stayed set — which the original "both or neither" constraint
// rejected, making the account undeletable. Bans and GDPR erasure both need
// this to work; 0025 relaxed the constraint to one direction.
func TestDeletingRedeemerKeepsInviteSpent(t *testing.T) {
	srv, _ := newTestServerCfg(t, inviteOnly)
	c := &client{t: t, base: srv.URL, ip: "10.9.0.5"}

	seedInvite(t, "KAGE-EEEE-55", nil)
	name := fmt.Sprintf("inv_del_%d", os.Getpid())
	status, body := c.do("POST", "/api/auth/register", map[string]any{
		"username": name, "email": name + "@test.local",
		"password": "longpassword123", "inviteCode": "KAGE-EEEE-55",
	})
	c.mustStatus(201, status, body, "signup with a code")

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `DELETE FROM users WHERE username = $1`, name); err != nil {
		t.Fatalf("deleting a member who joined by invite must not fail: %v", err)
	}

	// the code must stay burnt — a deleted account cannot recycle its invite
	used, userID := inviteUsedBy(t, "KAGE-EEEE-55")
	if !used {
		t.Fatal("invite became reusable after its redeemer was deleted")
	}
	if userID != nil {
		t.Fatalf("expected the user reference to be cleared, got %d", *userID)
	}
}
