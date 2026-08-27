// Package config loads all runtime configuration from the environment.
// Same variables as the old Node backend so .env files carry over unchanged.
package config

import (
	"runtime"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"animekage/backend/internal/httpx"
)

type Config struct {
	DatabaseURL  string
	JWTSecret    string
	JWTExpiresIn time.Duration
	Port         int
	CORSOrigins  []string
	UploadsDir   string
	// GiphyAPIKey enables the GIF picker. Empty = the feature is off and the
	// endpoints answer 503, which the UI reads as "hide the button".
	GiphyAPIKey string
	// TrustedProxies lists the CIDRs whose X-Forwarded-For we believe. Empty
	// (the default) means believe nobody and attribute every request to its
	// RemoteAddr — correct for a directly exposed server, wrong behind Caddy,
	// where it would put the whole internet in one rate-limit bucket. Set it
	// to the reverse proxy's network in production.
	TrustedProxies []netip.Prefix
	// ContentHosts restricts which domains content links (the iframe srcs) may
	// point at — suffix match, so "filemoon.sx" covers subdomains. Empty means
	// any public https host.
	ContentHosts []string
	// AniskipBaseURL is the AniSkip API root — overridable so tests
	// can stub it.
	AniskipBaseURL string
	// StagingDir holds release-pipeline uploads — transient work
	// files, deleted at publish/reject and auto-expired after 30 days.
	StagingDir string
	// TranslatorReleaseCap bounds how many unpublished releases one translator
	// may have in flight. Staging is sized by translators × cap × file size, so
	// this is the only thing that makes disk usage predictable: without it a
	// team that translates faster than it verifies grows the queue for ever.
	// 0 disables the cap.
	TranslatorReleaseCap int
	// HardsubPreset/HardsubTune/HardsubCRF tune the optional subtitle burn
	//. See the parse site in Load for the measured speed/size table and
	// why veryfast+animation is the default.
	HardsubPreset string
	HardsubTune   string
	HardsubCRF    int
	// HardsubNice/HardsubThreads bound what a burn may take from the box.
	// Defaults keep one core free and run the encoder at the lowest priority,
	// so browsing never waits behind an encode.
	HardsubNice    int
	HardsubThreads int
	// PublishVideoGrace is how long a published release keeps its staged video
	// before it is deleted. The subtitle track is permanent; the video is a
	// working copy of someone else's file and is not ours to hoard. Short by
	// design — it exists to undo a mistake, not to archive.
	PublishVideoGrace time.Duration
	// AnthropicAPIKey enables auto-translate; empty disables the
	// feature (the endpoint returns 503). AnthropicBaseURL lets tests stub
	// the API; TranslateModel defaults to Claude Sonnet 5.
	AnthropicAPIKey  string
	AnthropicBaseURL string
	TranslateModel   string
	// MangadexBaseURL overrides the MangaDex API root — tests stub
	// it; empty means the real API.
	MangadexBaseURL string
	// R2* configures the Cloudflare R2 bucket for own scanlation pages
	//. All-or-nothing: the feature is off unless account, key
	// pair, bucket, and public URL are all set. R2Endpoint overrides the
	// account-derived endpoint so tests can stub the S3 API.
	R2AccountID string
	R2AccessKey string
	R2SecretKey string
	R2Bucket    string
	R2PublicURL string
	R2Endpoint  string
	// PublicURL is the site's own origin, used to build absolute links in
	// outgoing mail. It cannot be derived from the request: a reset link is
	// followed from an inbox, where the Host header of the API call that
	// created it means nothing, and trusting that header would let anyone
	// mint a reset link pointing at their own domain.
	PublicURL string
	// SMTP* / MailFrom configure outgoing mail. With SMTPHost
	// empty the sender logs instead of sending — fine for dev, checked at
	// startup for prod.
	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string
	MailFrom     string
	// PasswordResetTTL bounds how long a reset link works.
	PasswordResetTTL time.Duration
	// InviteOnly closes registration behind a Discord-minted code.
	// Off by default on purpose: turning it on is a deliberate launch step, and
	// a default-on flag would silently break dev signups and the test suite.
	InviteOnly bool
}

// Load reads configuration from the environment. It fails hard on a missing
// JWT secret in every environment — the old backend's silent fallback to a
// hardcoded dev secret is exactly the bug we are not porting.
func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		UploadsDir:       envOr("UPLOADS_DIR", "./uploads"),
		GiphyAPIKey:      envOr("GIPHY_API_KEY", ""),
		AniskipBaseURL:   envOr("ANISKIP_BASE_URL", "https://api.aniskip.com"),
		StagingDir:       envOr("STAGING_DIR", "./staging"),
		AnthropicAPIKey:  os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicBaseURL: os.Getenv("ANTHROPIC_BASE_URL"),
		MangadexBaseURL:  os.Getenv("MANGADEX_BASE_URL"),
		TranslateModel:   envOr("TRANSLATE_MODEL", "claude-sonnet-5"),
		R2AccountID:      os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKey:      os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretKey:      os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Bucket:         os.Getenv("R2_BUCKET"),
		R2PublicURL:      os.Getenv("R2_PUBLIC_URL"),
		R2Endpoint:       os.Getenv("R2_ENDPOINT"),
		PublicURL:        strings.TrimRight(envOr("PUBLIC_URL", "http://localhost:5173"), "/"),
		SMTPHost:         os.Getenv("SMTP_HOST"),
		SMTPUser:         os.Getenv("SMTP_USER"),
		SMTPPassword:     os.Getenv("SMTP_PASSWORD"),
		MailFrom:         envOr("MAIL_FROM", "Anime-Kage <no-reply@anime-kage.ro>"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required (no fallback, by design)")
	}

	expires, err := parseExpiry(envOr("JWT_EXPIRES_IN", "7d"))
	if err != nil {
		return nil, fmt.Errorf("invalid JWT_EXPIRES_IN: %w", err)
	}
	cfg.JWTExpiresIn = expires

	port, err := strconv.Atoi(envOr("PORT", "3000"))
	if err != nil {
		return nil, fmt.Errorf("invalid PORT: %w", err)
	}
	cfg.Port = port

	cap, err := strconv.Atoi(envOr("TRANSLATOR_RELEASE_CAP", "10"))
	if err != nil || cap < 0 {
		return nil, fmt.Errorf("invalid TRANSLATOR_RELEASE_CAP: must be a non-negative integer")
	}
	cfg.TranslatorReleaseCap = cap

	// Hardsub encode knobs. Env rather than constants because the
	// right answer is a tradeoff only the operator can make, and it is worth
	// being able to try one without a rebuild.
	//
	// Measured on this 4-core aarch64 box, 1080p, anime-like content, CRF 20:
	//
	//	medium                    1.99x realtime   619 KB/20s
	//	veryfast                  3.18x            513 KB
	//	veryfast +tune animation  2.77x            291 KB   <- default
	//	ultrafast                 4.53x           1258 KB
	//
	// veryfast+animation is the optimum because it wins on *both* axes against
	// medium: 1.39x faster and 2.1x smaller. The tune does the heavy lifting —
	// x264's animation tuning suits flat cel-shaded areas and hard line art.
	//
	// ultrafast is the trap: 4.5x the speed but 4.3x the size, and the artefact
	// has to go up the same slow uplink to the host afterwards, so end to end it
	// loses badly. Faster is not free.
	cfg.HardsubPreset = envOr("HARDSUB_PRESET", "veryfast")
	cfg.HardsubTune = envOr("HARDSUB_TUNE", "animation")
	hcrf, err := strconv.Atoi(envOr("HARDSUB_CRF", "20"))
	if err != nil || hcrf < 0 || hcrf > 51 {
		return nil, fmt.Errorf("invalid HARDSUB_CRF: must be 0..51")
	}
	cfg.HardsubCRF = hcrf

	hn, err := strconv.Atoi(envOr("HARDSUB_NICE", "19"))
	if err != nil || hn < 0 || hn > 19 {
		return nil, fmt.Errorf("invalid HARDSUB_NICE: must be 0..19")
	}
	cfg.HardsubNice = hn

	// Default: every core but one. On a 4-core box that is 3 for the encoder
	// and one that browsing can always have, which is the difference between
	// "slower during an encode" and "unresponsive during an encode".
	defThreads := runtime.NumCPU() - 1
	if defThreads < 1 {
		defThreads = 1
	}
	ht, err := strconv.Atoi(envOr("HARDSUB_THREADS", strconv.Itoa(defThreads)))
	if err != nil || ht < 0 {
		return nil, fmt.Errorf("invalid HARDSUB_THREADS: must be >= 0 (0 = let ffmpeg decide)")
	}
	cfg.HardsubThreads = ht

	grace, err := parseExpiry(envOr("PUBLISH_VIDEO_GRACE", "5m"))
	if err != nil {
		return nil, fmt.Errorf("invalid PUBLISH_VIDEO_GRACE: %w", err)
	}
	cfg.PublishVideoGrace = grace

	if raw := os.Getenv("CORS_ORIGIN"); raw != "" {
		for _, o := range strings.Split(raw, ",") {
			cfg.CORSOrigins = append(cfg.CORSOrigins, strings.TrimSpace(o))
		}
	} else {
		cfg.CORSOrigins = []string{"http://localhost:5173", "http://localhost:4173"}
	}

	smtpPort, err := strconv.Atoi(envOr("SMTP_PORT", "587"))
	if err != nil || smtpPort <= 0 {
		return nil, fmt.Errorf("invalid SMTP_PORT: must be a positive integer")
	}
	cfg.SMTPPort = smtpPort

	// Short by design. A reset link is a bearer credential sitting in an
	// inbox; an hour is long enough to act on and short enough that a mail
	// account compromised next week is not also a site compromise.
	resetTTL, err := parseExpiry(envOr("PASSWORD_RESET_TTL", "1h"))
	if err != nil || resetTTL <= 0 {
		return nil, fmt.Errorf("invalid PASSWORD_RESET_TTL: must be a positive duration")
	}
	cfg.PasswordResetTTL = resetTTL

	cfg.InviteOnly = envOr("INVITE_ONLY", "false") == "true"

	trusted, err := httpx.ParseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return nil, fmt.Errorf("invalid TRUSTED_PROXIES: %w", err)
	}
	cfg.TrustedProxies = trusted

	if raw := os.Getenv("CONTENT_HOSTS"); raw != "" {
		for _, h := range strings.Split(raw, ",") {
			if h = strings.TrimSpace(h); h != "" {
				cfg.ContentHosts = append(cfg.ContentHosts, strings.ToLower(h))
			}
		}
	}

	return cfg, nil
}

// DatabaseURL returns just DATABASE_URL, for CLI tools (populate, autoupdate,
// migrate) that talk to the database but never sign tokens — they must not
// demand a JWT secret to run.
func DatabaseURL() (string, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return "", fmt.Errorf("DATABASE_URL is required")
	}
	return url, nil
}

// BotConfig is the Discord invite bot's configuration. Separate
// from Load for the same reason DatabaseURL is: the bot talks to the
// database and to Discord, never signs tokens, and must not be made to
// invent a JWT secret to boot.
type BotConfig struct {
	DatabaseURL string
	Token       string
	// GuildID scopes the slash command to one server. Guild commands appear
	// instantly; global ones take up to an hour to propagate, which makes
	// developing against them miserable. Empty registers globally.
	GuildID string
	// ChannelID restricts where the command may be used — the "#invitații"
	// channel. Empty allows it anywhere the bot can see.
	ChannelID string
	// CommandName is configurable because Discord validates command names, and
	// so the name can be changed without a redeploy if it ever needs to be
	// changed without a code edit.
	CommandName string
	// InviteTTL is how long a minted code stays valid. 0 = never expires.
	InviteTTL time.Duration
	// QuotaWindow is the per-member rate limit — one code per window.
	QuotaWindow time.Duration
	// PublicURL is the site origin the bot links to in the invite board. Same
	// variable the API uses, so the two cannot point at different sites.
	PublicURL string
	// ProtectionChannelID is the honeypot channel: a channel whose
	// only message tells everyone not to post in it. Anyone who posts anyway is
	// muted and their last few minutes of messages are deleted server-wide.
	// Empty disables the whole feature, including the notice message.
	ProtectionChannelID string
	// MutedRoleID is the role the honeypot hands out. It must be a role the
	// server denies Send Messages with, and it must sit BELOW the bot's own
	// role or Discord refuses the assignment. Empty falls back to a native
	// timeout, which Discord caps at 28 days — see PurgeWindow's neighbours in
	// LoadBot for why that is only a fallback.
	MutedRoleID string
	// PurgeWindow is how far back the honeypot deletes the offender's messages
	// in every channel. Discord's bulk-delete endpoint refuses anything older
	// than 14 days, which is the hard ceiling here.
	PurgeWindow time.Duration
	// ModLogChannelID receives one line per mute. Empty logs to stdout only,
	// which is enough to work but means nobody sees a false positive until
	// somebody complains.
	ModLogChannelID string
}

func LoadBot() (*BotConfig, error) {
	cfg := &BotConfig{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Token:       os.Getenv("DISCORD_TOKEN"),
		GuildID:     os.Getenv("DISCORD_GUILD_ID"),
		ChannelID:   os.Getenv("DISCORD_INVITE_CHANNEL_ID"),
		// Plain ASCII, deliberately. Discord accepts "invitație" — ț is a letter
		// and passes the name rules — but members have to *type* it into the
		// command bar, and a diacritic nobody has on their keyboard layout is a
		// speed bump on the one command that is the front door to the site.
		CommandName: envOr("DISCORD_INVITE_COMMAND", "invitatie"),
		PublicURL:   strings.TrimRight(envOr("PUBLIC_URL", "https://anime-kage.ro"), "/"),

		ProtectionChannelID: os.Getenv("DISCORD_PROTECTION_CHANNEL_ID"),
		MutedRoleID:         os.Getenv("DISCORD_MUTED_ROLE_ID"),
		ModLogChannelID:     os.Getenv("DISCORD_MOD_LOG_CHANNEL_ID"),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("DISCORD_TOKEN is required")
	}

	ttl, err := parseExpiry(envOr("INVITE_TTL", "7d"))
	if err != nil {
		return nil, fmt.Errorf("invalid INVITE_TTL: %w", err)
	}
	cfg.InviteTTL = ttl

	window, err := parseExpiry(envOr("INVITE_QUOTA_WINDOW", "24h"))
	if err != nil || window <= 0 {
		return nil, fmt.Errorf("invalid INVITE_QUOTA_WINDOW: must be a positive duration")
	}
	cfg.QuotaWindow = window

	purge, err := parseExpiry(envOr("DISCORD_PROTECTION_PURGE_WINDOW", "5m"))
	if err != nil || purge <= 0 || purge > 14*24*time.Hour {
		return nil, fmt.Errorf("invalid DISCORD_PROTECTION_PURGE_WINDOW: must be a positive duration no longer than 14 days (Discord's bulk-delete limit)")
	}
	cfg.PurgeWindow = purge

	// Pointing both at the same channel would mute everybody who asked for an
	// invite. Cheap to check here, unrecoverable if it ships.
	if cfg.ProtectionChannelID != "" && cfg.ProtectionChannelID == cfg.ChannelID {
		return nil, fmt.Errorf("DISCORD_PROTECTION_CHANNEL_ID must not be the invite channel")
	}

	return cfg, nil
}

// parseExpiry accepts Go durations ("168h") plus the "7d" day shorthand the
// old backend used, so existing .env values keep working.
func parseExpiry(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
