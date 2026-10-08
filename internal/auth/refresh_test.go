package auth

import (
	"encoding/base64"
	"testing"
)

func TestGenerateRefreshToken_Format(t *testing.T) {
	plain, hash, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	// 32 bytes, base64url without padding = 43 characters.
	if len(plain) != 43 {
		t.Fatalf("plain length = %d, want 43", len(plain))
	}
	// hex of a 32-byte digest = 64 characters.
	if len(hash) != 64 {
		t.Fatalf("hash length = %d, want 64", len(hash))
	}
	if _, err := base64.RawURLEncoding.DecodeString(plain); err != nil {
		t.Fatalf("plain is not valid base64url: %v", err)
	}
}

func TestGenerateRefreshToken_Unique(t *testing.T) {
	p1, _, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	p2, _, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if p1 == p2 {
		t.Fatal("two generated tokens are identical; entropy source broken")
	}
}

func TestGenerateRefreshToken_HashMatchesPlain(t *testing.T) {
	plain, hash, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}
	if HashRefreshToken(plain) != hash {
		t.Fatal("HashRefreshToken(plain) does not match the returned hash")
	}
}

func TestHashRefreshToken_Deterministic(t *testing.T) {
	const token = "fixed-input-for-testing"
	if HashRefreshToken(token) != HashRefreshToken(token) {
		t.Fatal("same input produced different hashes")
	}
}

func TestHashRefreshToken_DifferentInputs(t *testing.T) {
	if HashRefreshToken("token-a") == HashRefreshToken("token-b") {
		t.Fatal("different inputs produced the same hash")
	}
}
