package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestCreateUser_Success(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	u, err := testStore.CreateUser(ctx, "alice@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == uuid.Nil {
		t.Fatal("user ID is nil")
	}
	if u.Email != "alice@example.com" {
		t.Fatalf("email = %q, want alice@example.com", u.Email)
	}
	if u.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	if _, err := testStore.CreateUser(ctx, "dup@example.com", "hash"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := testStore.CreateUser(ctx, "dup@example.com", "other-hash")
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestGetUserByEmail_Found(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	created, err := testStore.CreateUser(ctx, "bob@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := testStore.GetUserByEmail(ctx, "bob@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("IDs differ: %v vs %v", got.ID, created.ID)
	}
}

func TestGetUserByEmail_NotFound(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	_, err := testStore.GetUserByEmail(ctx, "nobody@example.com")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
