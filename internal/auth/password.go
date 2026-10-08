package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the work factor. 12 is the current recommendation for
// interactive logins: slow enough to make brute-force expensive, fast
// enough to keep login under ~250ms.
const bcryptCost = 12

// HashPassword returns the bcrypt hash of the given plaintext password.
// The hash embeds the salt and cost, so no separate storage is needed.
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword reports whether plain matches the stored hash.
// It returns a plain error (no wrapping) because bcrypt already
// signals mismatch distinctly and we do not want to leak timing.
func CheckPassword(hash, plain string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)); err != nil {
		return err
	}
	return nil
}

// ErrInvalidCredentials is the single error returned to clients on any
// authentication failure. Callers must not distinguish between "unknown
// email" and "wrong password" in responses, to prevent user enumeration.
var ErrInvalidCredentials = errors.New("invalid credentials")
