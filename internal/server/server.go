package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/config"
	"github.com/Carlos20052030/bookmarks-api/internal/handler"
	"github.com/Carlos20052030/bookmarks-api/internal/middleware"
	"github.com/Carlos20052030/bookmarks-api/internal/storage"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

type Server struct {
	http *http.Server
}

// New builds the HTTP server with routes, middleware chain and timeouts.
// The server is not started until Run is called.
func New(
	cfg config.ServerConfig,
	authH *handler.AuthHandler,
	bookmarkH *handler.BookmarkHandler,
	tagH *handler.TagHandler,
	apiKeyH *handler.APIKeyHandler,
	store *storage.Store, // <-- store para API Key auth
	jwtSecret []byte,
	loginLimiter *middleware.Limiter,
	registerLimiter *middleware.Limiter,
	emailLimiter *middleware.Limiter,
	logger *slog.Logger,
) *Server {
	mux := http.NewServeMux()
	requireAuth := middleware.Auth(jwtSecret, logger)
	requireAPIKey := middleware.APIKeyAuth(store, logger)

	// Rate limit per IP (normalized to /64 for IPv6) and per email.
	// Both layers must pass. IP blocks network-level abuse; the email
	// layer catches credential stuffing with rotating IPs.
	rateLimitLoginIP := middleware.RateLimit(loginLimiter, middleware.IPKey)
	rateLimitLoginEmail := middleware.RateLimit(emailLimiter, middleware.EmailKeyFromBody)
	rateLimitRegisterIP := middleware.RateLimit(registerLimiter, middleware.IPKey)
	rateLimitRegisterEmail := middleware.RateLimit(emailLimiter, middleware.EmailKeyFromBody)

	// Public routes.
	mux.HandleFunc("GET /health", handleHealth)
	mux.Handle("POST /auth/register",
		rateLimitRegisterIP(rateLimitRegisterEmail(http.HandlerFunc(authH.Register))))
	mux.Handle("POST /auth/login",
		rateLimitLoginIP(rateLimitLoginEmail(http.HandlerFunc(authH.Login))))
	mux.HandleFunc("POST /auth/refresh", authH.Refresh)
	mux.HandleFunc("POST /auth/logout", authH.Logout)

	// Protected: bookmarks.
	mux.Handle("POST /bookmarks", requireAuth(http.HandlerFunc(bookmarkH.Create)))
	mux.Handle("GET /bookmarks", requireAuth(http.HandlerFunc(bookmarkH.List)))
	mux.Handle("GET /bookmarks/{id}", requireAuth(http.HandlerFunc(bookmarkH.Get)))
	mux.Handle("PUT /bookmarks/{id}", requireAuth(http.HandlerFunc(bookmarkH.Update)))
	mux.Handle("DELETE /bookmarks/{id}", requireAuth(http.HandlerFunc(bookmarkH.Delete)))
	mux.Handle("POST /bookmarks/{id}/tags/{tagID}", requireAuth(http.HandlerFunc(bookmarkH.AttachTag)))
	mux.Handle("DELETE /bookmarks/{id}/tags/{tagID}", requireAuth(http.HandlerFunc(bookmarkH.DetachTag)))

	// Protected: tags.
	mux.Handle("POST /tags", requireAuth(http.HandlerFunc(tagH.Create)))
	mux.Handle("GET /tags", requireAuth(http.HandlerFunc(tagH.List)))
	mux.Handle("DELETE /tags/{id}", requireAuth(http.HandlerFunc(tagH.Delete)))

	// Protected: API Keys (NOVAS ROTAS AQUI)
	mux.Handle("POST /api-keys", requireAuth(http.HandlerFunc(apiKeyH.Create)))
	mux.Handle("GET /api-keys", requireAuth(http.HandlerFunc(apiKeyH.List)))
	mux.Handle("DELETE /api-keys/{id}", requireAuth(http.HandlerFunc(apiKeyH.Revoke)))

	// Test route protected by API Key auth
	mux.Handle("GET /api/me", requireAPIKey(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.UserIDFromContext(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"user_id": userID.String(),
		})
	})))

	handlerChain := middleware.Chain(mux,
		middleware.CORS,
		middleware.RequestID,
		middleware.Logging(logger),
		middleware.Recovery(logger),
	)

	return &Server{
		http: &http.Server{
			Addr:              fmt.Sprintf(":%d", cfg.Port),
			Handler:           handlerChain,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
	}
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		slog.Info("http server listening", "addr", s.http.Addr)
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
