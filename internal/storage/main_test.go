package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/Carlos20052030/bookmarks-api/internal/config"
)

var (
	testDB    *sql.DB
	testStore *Store
)

func TestMain(m *testing.M) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "test config:", err)
		os.Exit(1)
	}

	db, err := Open(context.Background(), cfg.Database.DSN())
	if err != nil {
		fmt.Fprintln(os.Stderr, "test open db:", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := Migrate(context.Background(), db); err != nil {
		fmt.Fprintln(os.Stderr, "test migrate:", err)
		os.Exit(1)
	}

	testDB = db
	testStore = NewStore(db)
	os.Exit(m.Run())
}

// cleanTables truncates all application tables and resets sequences.
// Called at the start of every test that touches the DB.
func cleanTables(t *testing.T) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(),
		`TRUNCATE TABLE refresh_tokens, users RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}
