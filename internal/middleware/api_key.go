package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/google/uuid"
)

const apiKeyHeader = "X-API-Key"
const apiKeyPrefix = "nuntix_live_"
const keyPrefixLength = 8 // Number of hex chars to use as prefix (after "nuntix_live_")

// APIKeyStore defines the interface needed by the API Key middleware.
type APIKeyStore interface {
	GetAPIKeysByPrefix(ctx context.Context, prefix string) ([]domain.APIKey, error)
}

// APIKeyAuth validates the API Key in the X-API-Key header and
// stores the user ID in the request context. On any failure it replies 401
// with a stable JSON body and never calls the next handler.
func APIKeyAuth(store APIKeyStore, logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get(apiKeyHeader)
			if raw == "" {
				writeAuthError(w)
				return
			}

			// Extract prefix from the raw key for efficient lookup
			// Key format: "nuntix_live_" + 64 hex chars
			// Prefix: "nuntix_live_" + first 8 hex chars
			if !strings.HasPrefix(raw, apiKeyPrefix) {
				writeAuthError(w)
				return
			}
			hexPart := strings.TrimPrefix(raw, apiKeyPrefix)
			if len(hexPart) < keyPrefixLength {
				writeAuthError(w)
				return
			}
			prefix := apiKeyPrefix + hexPart[:keyPrefixLength]

			// Look up candidate keys by prefix
			keys, err := store.GetAPIKeysByPrefix(r.Context(), prefix)
			if err != nil {
				writeAuthError(w)
				return
			}

			// Pre-hash the raw key with SHA-256 to match the stored bcrypt format
			// (bcrypt of SHA-256 hex string to handle bcrypt's 72-byte limit)
			shaHash := sha256.Sum256([]byte(raw))
			shaHex := hex.EncodeToString(shaHash[:])

			// Compare the pre-hashed key against each candidate's bcrypt hash
			authenticated := false
			var matchedUserID uuid.UUID
			for _, key := range keys {
				if err := bcrypt.CompareHashAndPassword([]byte(key.KeyHash), []byte(shaHex)); err == nil {
					authenticated = true
					matchedUserID = key.UserID
					break
				}
			}

			if !authenticated {
				writeAuthError(w)
				return
			}

			// Valid key - inject user ID into context
			ctx := context.WithValue(r.Context(), userIDKey, matchedUserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}