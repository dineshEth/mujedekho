package auth_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"site24x7/internal/auth"
)

func TestHashVerifyRoundtrip(t *testing.T) {
	hash, err := auth.HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("want PHC argon2id string, got %q", hash)
	}
	if !auth.VerifyPassword("correct horse battery staple", hash) {
		t.Fatal("correct password must verify")
	}
	if auth.VerifyPassword("wrong", hash) {
		t.Fatal("wrong password must not verify")
	}
	if auth.VerifyPassword("correct horse battery staple", "garbage") {
		t.Fatal("malformed stored hash must fail closed")
	}
}

func TestTokensSignVerify(t *testing.T) {
	tk := auth.NewTokens([]byte("test-secret-at-least-32-bytes-long!!"), time.Minute)
	id := uuid.New()
	token, err := tk.SignAccess(id, "user@example.com")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := tk.VerifyAccess(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != id.String() || claims.Email != "user@example.com" {
		t.Fatalf("claims mismatch: %+v", claims)
	}

	// Wrong key fails.
	other := auth.NewTokens([]byte("another-secret-also-32-bytes-long!!"), time.Minute)
	if _, err := other.VerifyAccess(token); err == nil {
		t.Fatal("token from different secret must not verify")
	}

	// Expired token fails.
	expiring := auth.NewTokens([]byte("test-secret-at-least-32-bytes-long!!"), -time.Minute)
	expired, _ := expiring.SignAccess(id, "user@example.com")
	if _, err := expiring.VerifyAccess(expired); err == nil {
		t.Fatal("expired token must not verify")
	}
}

func TestRefreshTokenFormat(t *testing.T) {
	plain, hash, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatalf("new refresh: %v", err)
	}
	if len(hash) < 16 || hash == plain {
		t.Fatalf("hash must differ from plaintext: %q vs %q", plain, hash)
	}
	if auth.HashRefreshToken(plain) != hash {
		t.Fatal("hash of presented token must match stored hash")
	}
}
