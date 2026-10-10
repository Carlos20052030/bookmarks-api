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
func New(cfg config.ServerConfig, auth *handler.AuthHandler, logger *slog.Logger) *Server {
	mux := http.NewServeMux()

	// Public routes.
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /auth/register", auth.Register)
	mux.HandleFunc("POST /auth/login", auth.Login)
	mux.HandleFunc("POST /auth/refresh", auth.Refresh)
	mux.HandleFunc("POST /auth/logout", auth.Logout)

	handlerChain := middleware.Chain(mux,
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
// On cancellation it performs a graceful shutdown within shutdownTimeout.
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
