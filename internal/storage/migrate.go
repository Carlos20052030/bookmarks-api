package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Carlos20052030/bookmarks-api/migrations"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate applies every pending migration using golang-migrate.
// It runs automatically on application startup.
func Migrate(ctx context.Context, db *sql.DB) error {
	// Handle transition from old custom migration system to golang-migrate
	// The old system created schema_migrations with (name, applied_at) columns
	// golang-migrate expects (version, dirty) columns
	if err := migrateLegacySchema(ctx, db); err != nil {
		return fmt.Errorf("migrate legacy schema: %w", err)
	}

	// Create iofs source from embedded migrations
	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("create iofs source: %w", err)
	}

	// Create postgres database driver
	dbDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create postgres driver: %w", err)
	}

	// Create migrate instance
	m, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}

	// Run migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

// migrateLegacySchema handles the transition from the old custom migration system
// to golang-migrate. If the old schema_migrations table exists with the old schema,
// it drops it so golang-migrate can create its own table.
func migrateLegacySchema(ctx context.Context, db *sql.DB) error {
	// Check if the old schema_migrations table exists with the old schema
	// by trying to select from it with the old column names
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'schema_migrations' AND column_name = 'name'`).Scan(&count)
	if err != nil {
		// If query fails, table might not exist, which is fine
		return nil
	}

	if count > 0 {
		// Old schema exists, drop it so golang-migrate can create its own
		_, err = db.ExecContext(ctx, `DROP TABLE IF EXISTS schema_migrations`)
		if err != nil {
			return fmt.Errorf("drop legacy schema_migrations table: %w", err)
		}
	}

	return nil
}