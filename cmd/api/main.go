package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Carlos20052030/bookmarks-api/internal/config"
	"github.com/Carlos20052030/bookmarks-api/internal/handler"
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

	store := storage.NewStore(db)
	authHandler := handler.NewAuthHandler(store, cfg.JWT.Secret, slog.Default())
	bookmarkHandler := handler.NewBookmarkHandler(store, slog.Default())

	srv := server.New(cfg.Server, authHandler, bookmarkHandler, cfg.JWT.Secret, slog.Default())
	if err := srv.Run(ctx); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}

	slog.Info("shutdown complete")
}