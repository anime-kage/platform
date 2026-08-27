package handler

// Password reset: request a link by email, redeem it for a new
// password. Both endpoints sit behind the auth rate limiter.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"animekage/backend/internal/auth"
	"animekage/backend/internal/httpx"
	"animekage/backend/internal/mail"
	"animekage/backend/internal/repo"
)

// clientIP records who asked for a reset, for abuse investigation only. Same
// resolution as the rate limiter (httpx.ClientIP): X-Forwarded-For is read only
// when the request came from a configured trusted proxy, because a forged value
// here would poison the very log you would be reading during an incident.
func (h *Handler) clientIP(r *http.Request) string {
	return httpx.ClientIP(r, h.cfg.TrustedProxies)
}

// POST /api/auth/forgot-password
//
// Always answers 200 with the same message, whether or not the address has an
// account. Anything else turns this into an oracle for "is X registered
// here?" — on a site whose whole premise is an invite-only community, that
// membership list is exactly what should not be enumerable.
func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))

	const sent = "Dacă există un cont cu acest email, ți-am trimis un link de resetare."

	if email == "" {
		httpx.Error(w, http.StatusBadRequest, "Emailul este obligatoriu")
		return
	}

	user, err := h.repo.UserByEmail(r.Context(), email)
	if errors.Is(err, repo.ErrNotFound) {
		// deliberately indistinguishable from the success path
		httpx.JSON(w, http.StatusOK, map[string]any{"message": sent})
		return
	}
	if err != nil {
		httpx.Internal(w, "forgot password", err)
		return
	}

	token, err := repo.NewResetToken()
	if err != nil {
		httpx.Internal(w, "forgot password", err)
		return
	}
	expires := time.Now().Add(h.cfg.PasswordResetTTL)
	if err := h.repo.CreatePasswordReset(r.Context(), user.ID, repo.HashResetToken(token), expires, h.clientIP(r)); err != nil {
		httpx.Internal(w, "forgot password", err)
		return
	}

	link := fmt.Sprintf("%s/reseteaza-parola?token=%s", h.cfg.PublicURL, token)
	if err := h.mail.Send(r.Context(), mail.Message{
		To:      user.Email,
		Subject: "Resetare parolă · Anime-Kage",
		Text:    resetEmailBody(user.Username, link, h.cfg.PasswordResetTTL),
	}); err != nil {
		// The token is already stored, so a send failure is not the user's
		// problem to solve by retrying — but we must not claim it was sent.
		httpx.Internal(w, "send reset email", err)
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{"message": sent})
}

// POST /api/auth/reset-password
func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" {
		httpx.Error(w, http.StatusBadRequest, "Link invalid sau incomplet")
		return
	}
	// Same rule as registration — one password policy, one implementation.
	if err := auth.ValidatePassword(body.Password); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		httpx.Internal(w, "reset password", err)
		return
	}

	userID, err := h.repo.ConsumePasswordReset(r.Context(), repo.HashResetToken(token), hash)
	switch {
	case errors.Is(err, repo.ErrResetUnknown):
		httpx.Error(w, http.StatusBadRequest, "Link invalid. Cere unul nou.")
		return
	case errors.Is(err, repo.ErrResetUsed):
		httpx.Error(w, http.StatusBadRequest, "Linkul a fost deja folosit. Cere unul nou.")
		return
	case errors.Is(err, repo.ErrResetExpired):
		httpx.Error(w, http.StatusBadRequest, "Linkul a expirat. Cere unul nou.")
		return
	case err != nil:
		httpx.Internal(w, "reset password", err)
		return
	}

	// No token is issued here on purpose: whoever clicked the link proved
	// control of the inbox, not of the account, and signing them straight in
	// would make a compromised mailbox a one-click session. They log in.
	_ = userID
	httpx.JSON(w, http.StatusOK, map[string]any{
		"message": "Parola a fost schimbată. Te poți autentifica acum.",
	})
}

func resetEmailBody(username, link string, ttl time.Duration) string {
	return fmt.Sprintf(`Salut, %s!

Ai cerut resetarea parolei pentru contul tău Anime-Kage.
Deschide linkul de mai jos ca să îți alegi o parolă nouă:

%s

Linkul este valabil %s și poate fi folosit o singură dată.

Dacă nu tu ai cerut resetarea, ignoră acest email — parola ta rămâne neschimbată.

— Anime-Kage
`, username, link, humanTTL(ttl.String()))
}

// humanTTL turns a Go duration string into something readable in Romanian
// for the handful of values this setting realistically takes.
func humanTTL(d string) string {
	switch d {
	case "1h0m0s":
		return "o oră"
	case "30m0s":
		return "30 de minute"
	case "2h0m0s":
		return "două ore"
	case "24h0m0s":
		return "24 de ore"
	default:
		return d
	}
}
