package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Carlos20052030/bookmarks-api/internal/auth"
	"github.com/google/uuid"
)

const bearerPrefix = "Bearer "

var userIDKey = &contextKey{name: "user-id"}

// Auth validates the Bearer access token in the Authorization header and
// stores the user ID in the request context. On any failure it replies 401
// with a stable JSON body and never calls the next handler.
func Auth(secret []byte, logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, bearerPrefix) {
				writeAuthError(w)
				return
			}
			raw := strings.TrimPrefix(header, bearerPrefix)
			if raw == "" {
				writeAuthError(w)
				return
			}

			userID, err := auth.ParseAccessToken(raw, secret)
			if err != nil {
				writeAuthError(w)
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext returns the user ID stored by Auth, or uuid.Nil
// when the request is not authenticated.
func UserIDFromContext(ctx context.Context) uuid.UUID {
	if v, ok := ctx.Value(userIDKey).(uuid.UUID); ok {
		return v
	}
	return uuid.Nil
}

// writeAuthError emits a minimal JSON 401 without leaking whether the
// header was missing, malformed, or the token was simply invalid.
func writeAuthError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
}
