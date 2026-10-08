package auth

import (
	"strings"
	"testing"
)

func TestHashPassword_ProducesVerifiableHash(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" {
		t.Fatal("hash is empty")
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("hash does not look like bcrypt: %q", hash[:3])
	}
	if err := CheckPassword(hash, "correct horse battery staple"); err != nil {
		t.Fatalf("CheckPassword on correct password: %v", err)
	}
}

func TestHashPassword_DifferentSaltsPerCall(t *testing.T) {
	h1, err := HashPassword("same")
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	h2, err := HashPassword("same")
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if h1 == h2 {
		t.Fatal("two hashes of the same password are identical; salt not random")
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("correct")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := CheckPassword(hash, "wrong"); err == nil {
		t.Fatal("CheckPassword accepted a wrong password")
	}
}

func TestCheckPassword_MalformedHash(t *testing.T) {
	if err := CheckPassword("not-a-bcrypt-hash", "anything"); err == nil {
		t.Fatal("CheckPassword accepted a malformed hash")
	}
}
