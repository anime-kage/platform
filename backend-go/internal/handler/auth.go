package handler

import (
	"errors"
	"net/http"
	"strings"

	"animekage/backend/internal/auth"
	"animekage/backend/internal/httpx"
	"animekage/backend/internal/middleware"
	"animekage/backend/internal/model"
	"animekage/backend/internal/repo"
)

// POST /api/auth/register
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username   string `json:"username"`
		Email      string `json:"email"`
		Password   string `json:"password"`
		InviteCode string `json:"inviteCode"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	// Normalised on the way in so the address is stored in one canonical form
	// and every later lookup matches regardless of what case the client sent.
	// Done before validation so a trailing space from autofill is trimmed
	// rather than rejected.
	email := strings.ToLower(strings.TrimSpace(body.Email))

	if err := auth.ValidateRegister(body.Username, email, body.Password); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	// Codes are stored upper-case; nobody typing KAGE-7f2a-9x off a phone
	// should be told their invitation is invalid.
	inviteCode := strings.ToUpper(strings.TrimSpace(body.InviteCode))
	if h.cfg.InviteOnly && inviteCode == "" {
		httpx.Error(w, http.StatusBadRequest, "Ai nevoie de un cod de invitație — cere unul pe Discord cu /invitatie")
		return
	}

	if taken, err := h.repo.EmailTaken(r.Context(), email); err != nil {
		httpx.Internal(w, "register", err)
		return
	} else if taken {
		httpx.Error(w, http.StatusBadRequest, "User with this email already exists")
		return
	}
	if taken, err := h.repo.UsernameTaken(r.Context(), body.Username); err != nil {
		httpx.Internal(w, "register", err)
		return
	} else if taken {
		httpx.Error(w, http.StatusBadRequest, "Username is already taken")
		return
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		httpx.Internal(w, "register", err)
		return
	}
	// With the gate on, account creation and spending the code are one
	// transaction — see CreateUserWithInvite for why the ordering matters.
	var user *model.User
	if h.cfg.InviteOnly {
		user, err = h.repo.CreateUserWithInvite(r.Context(), body.Username, email, hash, inviteCode)
	} else {
		user, err = h.repo.CreateUser(r.Context(), body.Username, email, hash)
	}
	if err != nil {
		switch {
		case errors.Is(err, repo.ErrInviteUnknown):
			httpx.Error(w, http.StatusBadRequest, "Codul de invitație nu există")
			return
		case errors.Is(err, repo.ErrInviteUsed):
			httpx.Error(w, http.StatusBadRequest, "Codul de invitație a fost deja folosit")
			return
		case errors.Is(err, repo.ErrInviteExpired):
			httpx.Error(w, http.StatusBadRequest, "Codul de invitație a expirat — cere altul pe Discord")
			return
		case repo.IsUniqueViolation(err): // raced a concurrent signup
			httpx.Error(w, http.StatusBadRequest, "Username or email is already taken")
			return
		}
		httpx.Internal(w, "register", err)
		return
	}

	token, err := h.auth.Sign(user.ID, user.Username, user.Email, user.Role)
	if err != nil {
		httpx.Internal(w, "register", err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"message": "User registered successfully",
		"user":    user,
		"token":   token,
	})
}

// POST /api/auth/login
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Error(w, http.StatusBadRequest, "Invalid input data")
		return
	}
	// Addresses are stored lower-case, and the lookup is an exact match, so a
	// phone keyboard capitalising the first letter turned a correct password
	// into "Invalid email or password" -- the user typed the right thing, the
	// client changed it. Trimmed too, for addresses pasted with a space.
	email := strings.ToLower(strings.TrimSpace(body.Email))

	if err := auth.ValidateLogin(email, body.Password); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	userID, hash, err := h.repo.PasswordHashByEmail(r.Context(), email)
	if err != nil || !auth.CheckPassword(body.Password, hash) {
		// same message for unknown email and wrong password, like the old API
		httpx.Error(w, http.StatusUnauthorized, "Invalid email or password")
		return
	}

	// bans block new sessions here and writes at the endpoints —
	// already-issued JWTs stay stateless
	if banned, err := h.repo.IsUserBanned(r.Context(), userID); err != nil {
		httpx.Internal(w, "login", err)
		return
	} else if banned {
		httpx.Error(w, http.StatusForbidden, "Contul este suspendat.")
		return
	}

	user, err := h.repo.UserByID(r.Context(), userID)
	if err != nil {
		httpx.Internal(w, "login", err)
		return
	}
	token, err := h.auth.Sign(user.ID, user.Username, user.Email, user.Role)
	if err != nil {
		httpx.Internal(w, "login", err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"message": "Login successful",
		"user":    user,
		"token":   token,
	})
}

// GET /api/auth/me
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.UserFrom(r)
	user, err := h.repo.UserByID(r.Context(), claims.UserID)
	if err != nil {
		notFoundOr(w, err, "User not found", "fetch profile")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"user": user})
}

// POST /api/auth/logout — stateless JWT; the client just drops the token.
func (h *Handler) logout(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{"message": "Logout successful"})
}
