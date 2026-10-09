package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/google/uuid"
)

func TestCreateRefreshToken_Success(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	user := mustCreateUser(t, "u1@example.com")
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	tok, err := testStore.CreateRefreshToken(ctx, user.ID, "hash-a", expiresAt)
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	if tok.ID == uuid.Nil {
		t.Fatal("token ID is nil")
	}
	if tok.UserID != user.ID {
		t.Fatalf("UserID mismatch: %v vs %v", tok.UserID, user.ID)
	}
	if tok.RevokedAt != nil {
		t.Fatal("RevokedAt should be nil for a fresh token")
	}
}

func TestGetRefreshTokenByHash_Found(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	user := mustCreateUser(t, "u2@example.com")
	created, err := testStore.CreateRefreshToken(ctx, user.ID, "unique-hash", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := testStore.GetRefreshTokenByHash(ctx, "unique-hash")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("IDs differ")
	}
}

func TestGetRefreshTokenByHash_NotFound(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	_, err := testStore.GetRefreshTokenByHash(ctx, "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRevokeRefreshToken_SetsTimestamp(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	user := mustCreateUser(t, "u3@example.com")
	tok, err := testStore.CreateRefreshToken(ctx, user.ID, "to-revoke", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := testStore.RevokeRefreshToken(ctx, tok.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	got, err := testStore.GetRefreshTokenByHash(ctx, "to-revoke")
	if err != nil {
		t.Fatalf("Get after revoke: %v", err)
	}
	if got.RevokedAt == nil {
		t.Fatal("RevokedAt should be set after revocation")
	}
}

func TestRevokeRefreshToken_Idempotent(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	user := mustCreateUser(t, "u4@example.com")
	tok, err := testStore.CreateRefreshToken(ctx, user.ID, "idem", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := testStore.RevokeRefreshToken(ctx, tok.ID); err != nil {
		t.Fatalf("first revoke: %v", err)
	}
	if err := testStore.RevokeRefreshToken(ctx, tok.ID); err != nil {
		t.Fatalf("second revoke should be a no-op, got: %v", err)
	}
}

func mustCreateUser(t *testing.T, email string) domain.User {
	t.Helper()
	u, err := testStore.CreateUser(context.Background(), email, "hash")
	if err != nil {
		t.Fatalf("CreateUser(%q): %v", email, err)
	}
	return u
}
