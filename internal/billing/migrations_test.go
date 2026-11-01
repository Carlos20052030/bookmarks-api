package billing

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestMigrationsUpDown tests that all billing migrations can be applied and rolled back
func TestMigrationsUpDown(t *testing.T) {
	// Use test database URL from environment or default
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5434/postgres?sslmode=disable"
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Skip("skipping test; database not available")
	}

	// Find migration files
	migrationsDir := filepath.Join("..", "..", "migrations")
	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	if err != nil {
		t.Fatalf("failed to find migration files: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("no migration files found")
	}

	// Run each migration up
	for _, upFile := range files {
		t.Run("up_"+filepath.Base(upFile), func(t *testing.T) {
			sqlBytes, err := os.ReadFile(upFile)
			if err != nil {
				t.Fatalf("failed to read migration %s: %v", upFile, err)
			}
			_, err = db.Exec(string(sqlBytes))
			if err != nil {
				t.Fatalf("failed to apply migration %s: %v", upFile, err)
			}
		})
	}

	// Verify schema after all migrations
	t.Run("verify_schema", func(t *testing.T) {
		verifyBillingSchema(t, db)
	})

	// Run each migration down (in reverse order)
	for i := len(files) - 1; i >= 0; i-- {
		upFile := files[i]
		downFile := upFile[:len(upFile)-len(".up.sql")] + ".down.sql"
		t.Run("down_"+filepath.Base(downFile), func(t *testing.T) {
			sqlBytes, err := os.ReadFile(downFile)
			if err != nil {
				t.Fatalf("failed to read migration %s: %v", downFile, err)
			}
			_, err = db.Exec(string(sqlBytes))
			if err != nil {
				t.Fatalf("failed to rollback migration %s: %v", downFile, err)
			}
		})
	}
}

// verifyBillingSchema checks that the billing schema is correct after migrations
func verifyBillingSchema(t *testing.T, db *sql.DB) {
	// Check users table has billing columns
	rows, err := db.Query(`
		SELECT column_name, data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_name = 'users' AND column_name IN ('plan', 'subscription_status', 'stripe_customer_id', 'current_period_end')
		ORDER BY column_name
	`)
	if err != nil {
		t.Fatalf("failed to query users columns: %v", err)
	}
	defer rows.Close()

	expectedUsersColumns := map[string]struct {
		dataType     string
		isNullable   string
		columnDefault string
	}{
		"plan":               {"text", "NO", "'free'"},
		"subscription_status": {"text", "NO", "'none'"},
		"stripe_customer_id": {"text", "YES", "NULL"},
		"current_period_end": {"timestamp with time zone", "YES", "NULL"},
	}

	found := make(map[string]bool)
	for rows.Next() {
		var colName, dataType, isNullable, colDefault string
		if err := rows.Scan(&colName, &dataType, &isNullable, &colDefault); err != nil {
			t.Fatalf("failed to scan column: %v", err)
		}
		found[colName] = true
		exp := expectedUsersColumns[colName]
		if dataType != exp.dataType {
			t.Errorf("column %s: expected data_type %s, got %s", colName, exp.dataType, dataType)
		}
		if isNullable != exp.isNullable {
			t.Errorf("column %s: expected is_nullable %s, got %s", colName, exp.isNullable, isNullable)
		}
		if colDefault != exp.columnDefault {
			t.Errorf("column %s: expected default %s, got %s", colName, exp.columnDefault, colDefault)
		}
	}

	for colName := range expectedUsersColumns {
		if !found[colName] {
			t.Errorf("missing column in users table: %s", colName)
		}
	}

	// Check users table has unique index on stripe_customer_id
	idxRows, err := db.Query(`
		SELECT indexname, indexdef
		FROM pg_indexes
		WHERE tablename = 'users' AND indexname = 'idx_users_stripe_customer_id'
	`)
	if err != nil {
		t.Fatalf("failed to query indexes: %v", err)
	}
	defer idxRows.Close()

	if !idxRows.Next() {
		t.Error("missing unique index idx_users_stripe_customer_id on users.stripe_customer_id")
	}

	// Check stripe_events table exists
	var exists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'stripe_events'
		)
	`).Scan(&exists)
	if err != nil {
		t.Fatalf("failed to check stripe_events table: %v", err)
	}
	if !exists {
		t.Error("stripe_events table does not exist")
	}

	// Check stripe_events columns
	evtRows, err := db.Query(`
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_name = 'stripe_events'
		ORDER BY column_name
	`)
	if err != nil {
		t.Fatalf("failed to query stripe_events columns: %v", err)
	}
	defer evtRows.Close()

	expectedEventColumns := map[string]struct {
		dataType   string
		isNullable string
	}{
		"id":            {"bigint", "NO"},
		"event_id":      {"text", "NO"},
		"event_type":    {"text", "NO"},
		"payload_json":  {"jsonb", "NO"},
		"processed_at":  {"timestamp with time zone", "NO"},
		"success":       {"boolean", "NO"},
	}

	foundEvents := make(map[string]bool)
	for evtRows.Next() {
		var colName, dataType, isNullable string
		if err := evtRows.Scan(&colName, &dataType, &isNullable); err != nil {
			t.Fatalf("failed to scan event column: %v", err)
		}
		foundEvents[colName] = true
		exp := expectedEventColumns[colName]
		if dataType != exp.dataType {
			t.Errorf("stripe_events column %s: expected data_type %s, got %s", colName, exp.dataType, dataType)
		}
		if isNullable != exp.isNullable {
			t.Errorf("stripe_events column %s: expected is_nullable %s, got %s", colName, exp.isNullable, isNullable)
		}
	}

	for colName := range expectedEventColumns {
		if !foundEvents[colName] {
			t.Errorf("missing column in stripe_events table: %s", colName)
		}
	}

	// Check unique index on stripe_events.event_id
	evtIdxRows, err := db.Query(`
		SELECT indexname
		FROM pg_indexes
		WHERE tablename = 'stripe_events' AND indexname = 'idx_stripe_events_event_id'
	`)
	if err != nil {
		t.Fatalf("failed to query stripe_events indexes: %v", err)
	}
	defer evtIdxRows.Close()

	if !evtIdxRows.Next() {
		t.Error("missing unique index idx_stripe_events_event_id on stripe_events.event_id")
	}
}