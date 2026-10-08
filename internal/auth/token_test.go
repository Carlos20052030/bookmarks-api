package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var testSecret = []byte("test-secret-at-least-32-bytes-long!!")

func TestIssueAndParse_RoundTrip(t *testing.T) {
	userID := uuid.New()

	token, err := IssueAccessToken(userID, testSecret)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	parsed, err := ParseAccessToken(token, testSecret)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	if parsed != userID {
		t.Fatalf("parsed userID = %v, want %v", parsed, userID)
	}
}

func TestParse_WrongSecret(t *testing.T) {
	userID := uuid.New()
	token, err := IssueAccessToken(userID, testSecret)
	if err != nil {
		t.Fatalf("IssueAccessToken: %v", err)
	}

	otherSecret := []byte("a-completely-different-secret-32b!")
	_, err = ParseAccessToken(token, otherSecret)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestParse_ExpiredToken(t *testing.T) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now.Add(-1 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-30 * time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(testSecret)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = ParseAccessToken(signed, testSecret)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for expired token, got %v", err)
	}
}

func TestParse_AlgorithmConfusion(t *testing.T) {
	// A token signed with "none" must be rejected.
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.New().String(),
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}

	_, err = ParseAccessToken(signed, testSecret)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken for none-alg token, got %v", err)
	}
}

func TestParse_MalformedToken(t *testing.T) {
	_, err := ParseAccessToken("not.a.jwt", testSecret)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}
