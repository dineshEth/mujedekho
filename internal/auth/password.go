// Package auth implements the security primitives shared by the API and
// tooling: argon2id password hashing, JWT access tokens, and refresh token
// rotation.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. 64MiB of memory per hash call is interactive-use
// friendly for a control plane while staying strong; tune upward if hardware
// allows.
const (
	argonMemory  = 64 * 1024
	argonIters   = 3
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
	// On creation the plaintext is never longer than this.
	MaxPasswordLength = 256
)

// ErrInvalidHash means the stored hash could not be parsed; treat as failed
// authentication, never as a reason to bypass.
var ErrInvalidHash = errors.New("auth: malformed password hash")

// HashPassword returns a PHC-formatted argon2id hash.
func HashPassword(password string) (string, error) {
	if len(password) > MaxPasswordLength {
		return "", fmt.Errorf("auth: password too long")
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonIters, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIters, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword checks a plaintext against a stored PHC hash in constant
// time relative to hash correctness.
func VerifyPassword(password, stored string) bool {
	fields := strings.Split(stored, "$")
	if len(fields) != 6 || fields[1] != "argon2id" {
		return false
	}
	var version int
	if _, err := fmt.Sscanf(fields[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	var mem, iters, threads int
	if _, err := fmt.Sscanf(fields[3], "m=%d,t=%d,p=%d", &mem, &iters, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(fields[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(fields[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(iters), uint32(mem), uint8(threads), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
