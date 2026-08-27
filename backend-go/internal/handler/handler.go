// Package handler is the HTTP layer: request parsing, validation, and the
// exact response shapes of the old Node backend. Business rules live here or
// in repo — nothing else knows about HTTP.
package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"animekage/backend/internal/anilist"
	"animekage/backend/internal/aniskip"
	"animekage/backend/internal/auth"
	"animekage/backend/internal/config"
	"animekage/backend/internal/giphy"
	"animekage/backend/internal/httpx"
	"animekage/backend/internal/jikan"
	"animekage/backend/internal/mail"
	"animekage/backend/internal/mangaext"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/repo"
	"animekage/backend/internal/resolver"
	"animekage/backend/internal/storage"
	"animekage/backend/internal/translate"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	repo    *repo.Repo
	auth    *auth.Manager
	jikan   *jikan.Client
	anilist *anilist.Client // fallback metadata source when Jikan is down
	// after a hard Jikan failure, skip it for a while so the AniList
	// fallback answers instantly instead of after Jikan's retry budget
	jikanDownUntil atomic.Int64 // unix seconds
	resolver       *resolver.Registry
	mangaext       *mangaext.Registry
	pageCache      pageSetCache
	aniskip        *aniskip.Client
	giphy          *giphy.Client
	skipMisses     skipMissCache
	// translator is nil when ANTHROPIC_API_KEY is unset — auto-translate 503s
	translator  *translate.Translator
	translating sync.Map // release id → *translationStatus (progress + result)
	// hardsubbing holds the live encode fraction for burns in flight.
	// Same shape and same reasoning as translating: it moves several times a
	// second and nobody needs it after the fact, so it stays out of the database.
	hardsubbing sync.Map // release id → *hardsubProgress
	// remuxing is the same thing for MKV→MP4 rewraps at ingest. A
	// separate map, not a separate progress type: the two jobs report the same
	// shape and share the worker, so they share the bookkeeping.
	remuxing sync.Map // release id → *hardsubProgress
	// storage is nil when R2 is unconfigured — page uploads fall back to
	// local UPLOADS_DIR
	storage *storage.Client
	// live chat: in-process SSE fan-out plus a per-user send
	// throttle. Both are per-instance state — see chathub.go on scaling out.
	chat      *chatHub
	chatLimit *chatLimiter
	// mail sends password resets. Never nil — with SMTP unconfigured it logs
	// the message instead, which is what makes reset testable in dev.
	mail *mail.Sender
	cfg  *config.Config
	pool *pgxpool.Pool // health check only
}

func New(pool *pgxpool.Pool, am *auth.Manager, cfg *config.Config) *Handler {
	base := cfg.AniskipBaseURL
	if base == "" {
		base = "https://api.aniskip.com"
	}
	h := &Handler{
		repo:      repo.New(pool),
		auth:      am,
		jikan:     jikan.NewClient(),
		anilist:   anilist.NewClient(),
		resolver:  resolver.Default(),
		mangaext:  mangaext.Default(cfg.MangadexBaseURL),
		aniskip:   aniskip.NewClient(base),
		giphy:     giphy.New(cfg.GiphyAPIKey),
		chat:      newChatHub(),
		chatLimit: newChatLimiter(),
		mail: mail.New(mail.Config{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUser,
			Password: cfg.SMTPPassword,
			From:     cfg.MailFrom,
		}),
		cfg:  cfg,
		pool: pool,
	}
	if !h.mail.Configured() {
		slog.Warn("SMTP not configured — password reset links will be logged, not emailed")
	}
	if cfg.AnthropicAPIKey != "" {
		h.translator = translate.New(cfg.AnthropicAPIKey, cfg.AnthropicBaseURL, cfg.TranslateModel)
	}
	if st, err := storage.New(cfg); err != nil {
		slog.Warn("R2 storage disabled", "error", err)
	} else {
		h.storage = st
	}
	return h
}

// errJikanSkipped marks a Jikan call we didn't make because the client is in
// its post-failure cooldown; callers fall through to AniList.
var errJikanSkipped = errors.New("jikan skipped: marked down")

func (h *Handler) jikanUp() bool {
	return time.Now().Unix() >= h.jikanDownUntil.Load()
}

func (h *Handler) noteJikanDown() {
	h.jikanDownUntil.Store(time.Now().Add(2 * time.Minute).Unix())
}

// viewerID returns the authenticated user's id, or nil for guests.
func viewerID(r *http.Request) *int {
	if u := middleware.UserFrom(r); u != nil {
		return &u.UserID
	}
	return nil
}

// notFoundOr maps repo.ErrNotFound to a 404 with msg, everything else to a
// logged 500 ("Failed to <action>").
func notFoundOr(w http.ResponseWriter, err error, msg, action string) {
	if errors.Is(err, repo.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, msg)
		return
	}
	httpx.Internal(w, action, err)
}
