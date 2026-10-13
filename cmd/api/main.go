package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/config"
	"github.com/Carlos20052030/bookmarks-api/internal/fetcher"
	"github.com/Carlos20052030/bookmarks-api/internal/handler"
	"github.com/Carlos20052030/bookmarks-api/internal/middleware"
	"github.com/Carlos20052030/bookmarks-api/internal/server"
	"github.com/Carlos20052030/bookmarks-api/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := storage.Open(ctx, cfg.Database.DSN())
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := storage.Migrate(ctx, db); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}

	// Rate limits: three buckets, all must pass.
	//
	//   loginIP      — 10/min per client IP (normalized to /64 for IPv6)
	//   registerIP   — 5/min per client IP
	//   email        — 10/min per email address, applies to login and register
	//
	// The email bucket is what closes the door on credential stuffing from
	// a rotating IP pool: even if the attacker rotates IPs, the target
	// account still gets 10 attempts per minute.
	loginLimiter := middleware.NewLimiter(10, 10.0/60.0)
	loginLimiter.StartCleaner(ctx)

	registerLimiter := middleware.NewLimiter(5, 5.0/60.0)
	registerLimiter.StartCleaner(ctx)

	emailLimiter := middleware.NewLimiter(10, 10.0/60.0)
	emailLimiter.StartCleaner(ctx)

	titleFetcher := fetcher.New(5 * time.Second)

	store := storage.NewStore(db)
	authHandler := handler.NewAuthHandler(store, cfg.JWT.Secret, slog.Default())
	bookmarkHandler := handler.NewBookmarkHandler(store, titleFetcher, slog.Default())
	tagHandler := handler.NewTagHandler(store, slog.Default())

	srv := server.New(
		cfg.Server,
		authHandler,
		bookmarkHandler,
		tagHandler,
		cfg.JWT.Secret,
		loginLimiter,
		registerLimiter,
		emailLimiter,
		slog.Default(),
	)
	if err := srv.Run(ctx); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}

	slog.Info("shutdown complete")
}