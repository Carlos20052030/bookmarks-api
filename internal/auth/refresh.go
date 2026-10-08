package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"
)

// RefreshTokenTTL is intentionally long: the access token is short-lived
// and refreshed through this token, which is revocable server-side.
const RefreshTokenTTL = 7 * 24 * time.Hour

// refreshTokenBytes is the entropy size. 32 bytes = 256 bits. At this
// size, brute-forcing the token is infeasible even with unlimited time.
const refreshTokenBytes = 32

// GenerateRefreshToken returns a new opaque refresh token.
// The plain value is sent to the client exactly once and never stored.
// The hash is what goes into the database.
func GenerateRefreshToken() (plain, hash string, err error) {
	buf := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(buf)
	hash = HashRefreshToken(plain)
	return plain, hash, nil
}

// HashRefreshToken returns the hex-encoded SHA-256 of the token.
//
// SHA-256 (not bcrypt) is correct here: the token already carries 256 bits
// of entropy from crypto/rand, so slow hashing adds no meaningful security
// — it would only slow down every verification. bcrypt is for low-entropy
// secrets like user passwords.
func HashRefreshToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}