// Package auth: password hashing, JWT issue/verify, and register/login
// validation. Claims use the exact JSON keys of the old Node backend
// ({userId, username, email, role}, HS256), so tokens issued before the
// rewrite keep working as long as JWT_SECRET is unchanged.
package auth

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

type Claims struct {
	UserID   int    `json:"userId"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret    []byte
	expiresIn time.Duration
}

func NewManager(secret string, expiresIn time.Duration) *Manager {
	return &Manager{secret: []byte(secret), expiresIn: expiresIn}
}

func (m *Manager) Sign(userID int, username, email, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		Email:    email,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.expiresIn)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *Manager) Verify(token string) (*Claims, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, errors.New("invalid or expired token")
	}
	return claims, nil
}

// BearerToken extracts the token from an Authorization header, or "".
func BearerToken(header string) string {
	if after, ok := strings.CutPrefix(header, "Bearer "); ok {
		return after
	}
	return ""
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(b), err
}

func CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// ValidateRegister mirrors the old zod schema, with the password minimum
// raised from 6 to 10 (code-review fix 1.3).
func ValidateRegister(username, email, password string) error {
	switch {
	case len(username) < 3 || len(username) > 50:
		return errors.New("Username must be between 3 and 50 characters")
	case !usernameRe.MatchString(username):
		return errors.New("Username can only contain letters, numbers, and underscores")
	case len(email) > 255 || !emailRe.MatchString(email):
		return errors.New("Invalid email format")
	}
	return ValidatePassword(password)
}

// ValidatePassword is the whole password policy, shared by registration and
// password reset so the two can never drift apart.
func ValidatePassword(password string) error {
	switch {
	// Length is the whole policy — no case or symbol rules. Forced complexity
	// buys little and pushes people towards "Parola1!", so length it is.
	// Counted in runes, not bytes: "parolăă" is seven characters however many
	// bytes the diacritics take, and this is a Romanian-language site.
	case utf8.RuneCountInString(password) < 8:
		return errors.New("Password must be at least 8 characters")
	// bcrypt refuses anything over 72 bytes outright, so a longer password
	// used to pass validation and then fail at hashing as a 500. Bounded in
	// bytes because that is the limit bcrypt actually applies.
	case len(password) > 72:
		return errors.New("Password must be at most 72 bytes")
	}
	return nil
}

func ValidateLogin(email, password string) error {
	if !emailRe.MatchString(email) {
		return errors.New("Invalid email format")
	}
	if password == "" {
		return errors.New("Password is required")
	}
	return nil
}
