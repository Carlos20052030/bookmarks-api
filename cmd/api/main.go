package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/Carlos20052030/bookmarks-api/internal/config"
	"github.com/Carlos20052030/bookmarks-api/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()

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

	slog.Info("api starting", "port", cfg.Server.Port)
}
