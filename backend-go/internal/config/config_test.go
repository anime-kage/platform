package config

import (
	"testing"
	"time"
)

// setBase gives Load the two required vars; individual tests override the rest.
func setBase(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgresql://u:p@localhost:5432/db")
	t.Setenv("JWT_SECRET", "s3cret")
	// neutralize anything inherited from the developer's shell
	for _, k := range []string{"JWT_EXPIRES_IN", "PORT", "CORS_ORIGIN", "UPLOADS_DIR"} {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	setBase(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.JWTExpiresIn != 7*24*time.Hour {
		t.Errorf("JWTExpiresIn = %v, want 168h", cfg.JWTExpiresIn)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[0] != "http://localhost:5173" {
		t.Errorf("CORSOrigins = %v", cfg.CORSOrigins)
	}
	if cfg.UploadsDir != "./uploads" {
		t.Errorf("UploadsDir = %q", cfg.UploadsDir)
	}
}

func TestLoadFailsHardOnMissingSecrets(t *testing.T) {
	// the whole point of the rewrite's config: no fallbacks, ever
	setBase(t)
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load succeeded without JWT_SECRET")
	}

	setBase(t)
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load succeeded without DATABASE_URL")
	}
}

func TestLoadExpiry(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"7d", 168 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"12h", 12 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"never", 0, true},
		{"xd", 0, true},
	}
	for _, tt := range tests {
		setBase(t)
		t.Setenv("JWT_EXPIRES_IN", tt.in)
		cfg, err := Load()
		if (err != nil) != tt.wantErr {
			t.Errorf("JWT_EXPIRES_IN=%q: err=%v, wantErr=%v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && cfg.JWTExpiresIn != tt.want {
			t.Errorf("JWT_EXPIRES_IN=%q: got %v, want %v", tt.in, cfg.JWTExpiresIn, tt.want)
		}
	}
}

func TestLoadCORSSplitting(t *testing.T) {
	setBase(t)
	t.Setenv("CORS_ORIGIN", "https://a.example, https://b.example ,https://c.example")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"https://a.example", "https://b.example", "https://c.example"}
	if len(cfg.CORSOrigins) != len(want) {
		t.Fatalf("CORSOrigins = %v, want %v", cfg.CORSOrigins, want)
	}
	for i := range want {
		if cfg.CORSOrigins[i] != want[i] {
			t.Errorf("CORSOrigins[%d] = %q, want %q", i, cfg.CORSOrigins[i], want[i])
		}
	}
}

func TestLoadBadPort(t *testing.T) {
	setBase(t)
	t.Setenv("PORT", "not-a-port")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted a non-numeric PORT")
	}
}

// setBotBase gives LoadBot the two vars it refuses to start without and clears
// everything else the developer's shell might be holding.
func setBotBase(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgresql://u:p@localhost:5432/db")
	t.Setenv("DISCORD_TOKEN", "token")
	for _, k := range []string{
		"DISCORD_GUILD_ID", "DISCORD_INVITE_CHANNEL_ID", "DISCORD_INVITE_COMMAND",
		"INVITE_TTL", "INVITE_QUOTA_WINDOW", "DISCORD_PROTECTION_CHANNEL_ID",
		"DISCORD_MUTED_ROLE_ID", "DISCORD_MOD_LOG_CHANNEL_ID",
		"DISCORD_PROTECTION_PURGE_WINDOW",
	} {
		t.Setenv(k, "")
	}
}

func TestLoadBotProtectionDefaults(t *testing.T) {
	setBotBase(t)
	cfg, err := LoadBot()
	if err != nil {
		t.Fatalf("LoadBot: %v", err)
	}
	if cfg.PurgeWindow != 5*time.Minute {
		t.Errorf("PurgeWindow = %v, want 5m", cfg.PurgeWindow)
	}
	// Absent means off: no channel, no honeypot, no accidental muting on a
	// deploy that has not been told which channel is the trap.
	if cfg.ProtectionChannelID != "" || cfg.MutedRoleID != "" || cfg.ModLogChannelID != "" {
		t.Errorf("protection is on by default: %+v", cfg)
	}
}

func TestLoadBotRejectsHoneypotOnTheInviteChannel(t *testing.T) {
	// The misconfiguration that would mute everybody who came for an invite.
	setBotBase(t)
	t.Setenv("DISCORD_INVITE_CHANNEL_ID", "123")
	t.Setenv("DISCORD_PROTECTION_CHANNEL_ID", "123")
	if _, err := LoadBot(); err == nil {
		t.Fatal("LoadBot accepted the invite channel as the honeypot")
	}
}

func TestLoadBotRejectsUnusablePurgeWindow(t *testing.T) {
	// Anything past 14 days cannot be bulk-deleted, so a config that asks for
	// it would silently delete nothing.
	for _, v := range []string{"0", "-5m", "15d", "nonsense"} {
		setBotBase(t)
		t.Setenv("DISCORD_PROTECTION_PURGE_WINDOW", v)
		if _, err := LoadBot(); err == nil {
			t.Errorf("LoadBot accepted DISCORD_PROTECTION_PURGE_WINDOW=%q", v)
		}
	}
}
