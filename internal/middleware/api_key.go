package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
)

const apiKeyHeader = "X-API-Key"

// APIKeyStore defines the interface needed by the API Key middleware.
type APIKeyStore interface {
	GetAPIKeyByHash(ctx context.Context, hash string) (domain.APIKey, error)
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

			// Compute SHA-256 hash of the raw key
			hash := sha256.Sum256([]byte(raw))
			hashString := hex.EncodeToString(hash[:])

			// Look up the key in the database
			key, err := store.GetAPIKeyByHash(r.Context(), hashString)
			if err != nil {
				writeAuthError(w)
				return
			}

			// Key found but revoked
			if key.RevokedAt != nil {
				writeAuthError(w)
				return
			}

			// Valid key - inject user ID into context
			ctx := context.WithValue(r.Context(), userIDKey, key.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}