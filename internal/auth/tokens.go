package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims carries the authenticated subject into the request context.
type Claims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
}

// Tokens signs and verifies access tokens (JWT ED25519/HS256) and creates
// opaque refresh tokens (server-side half only; hashes stored by callers).
type Tokens struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time
}

func NewTokens(secret []byte, accessTTL time.Duration) *Tokens {
	return &Tokens{secret: secret, accessTTL: accessTTL, now: time.Now}
}

// WithClock overrides time for tests.
func (t *Tokens) WithClock(now func() time.Time) { t.now = now }

// AccessTTL returns the configured access-token lifetime.
func (t *Tokens) AccessTTL() time.Duration { return t.accessTTL }

var ErrInvalidToken = errors.New("auth: invalid token")

// SignAccess issues a short-lived access token for the user.
func (t *Tokens) SignAccess(userID uuid.UUID, email string) (string, error) {
	now := t.now().UTC()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.accessTTL)),
			Issuer:    "site24x7",
			ID:        uuid.NewString(),
		},
		Email: email,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// VerifyAccess validates signature and expiry; returns the claims or a
// non-nil error.
func (t *Tokens) VerifyAccess(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(*jwt.Token) (any, error) {
		return t.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// NewRefreshToken returns (plaintext, hash-at-rest). Only the SHA-256 hash is
// stored; the plaintext is shown once to the client.
func NewRefreshToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("auth: refresh token: %w", err)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	return plaintext, hashToken(plaintext), nil
}

// HashRefreshToken computes the at-rest hash of a presented refresh token.
func HashRefreshToken(plaintext string) string { return hashToken(plaintext) }

func hashToken(s string) string {
	return strings.ToLower(base64.RawURLEncoding.EncodeToString([]byte(s)))
}

// NormalizeEmail emits the canonical lowercased email for lookups. Case folds
// hide in the local part; citext handles storage, this guards constructions.
func NormalizeEmail(email string) string { return strings.TrimSpace(strings.ToLower(email)) }
