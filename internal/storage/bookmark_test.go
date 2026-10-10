package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestCreateBookmark_Success(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "bm1@example.com")

	b, err := testStore.CreateBookmark(ctx, user.ID, "https://example.com", "Example")
	if err != nil {
		t.Fatalf("CreateBookmark: %v", err)
	}
	if b.ID == uuid.Nil {
		t.Fatal("bookmark ID is nil")
	}
	if b.UserID != user.ID {
		t.Fatalf("UserID = %v, want %v", b.UserID, user.ID)
	}
	if b.URL != "https://example.com" {
		t.Fatalf("URL = %q", b.URL)
	}
}

func TestGetBookmarkByID_Found(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "bm2@example.com")
	created, _ := testStore.CreateBookmark(ctx, user.ID, "https://a.com", "A")

	got, err := testStore.GetBookmarkByID(ctx, user.ID, created.ID)
	if err != nil {
		t.Fatalf("GetBookmarkByID: %v", err)
	}
	if got.ID != created.ID {
		t.Fatal("IDs differ")
	}
}

func TestGetBookmarkByID_NotFound(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "bm3@example.com")

	_, err := testStore.GetBookmarkByID(ctx, user.ID, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestGetBookmarkByID_IDOR is the ownership test: user B must not be able
// to read user A's bookmark by guessing its ID.
func TestGetBookmarkByID_IDOR(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	alice := mustCreateUser(t, "alice@example.com")
	bob := mustCreateUser(t, "bob@example.com")

	aliceBookmark, _ := testStore.CreateBookmark(ctx, alice.ID, "https://alice.com", "Alice")

	_, err := testStore.GetBookmarkByID(ctx, bob.ID, aliceBookmark.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-user access, got %v", err)
	}
}

func TestListBookmarks_OnlyOwn(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	alice := mustCreateUser(t, "alice2@example.com")
	bob := mustCreateUser(t, "bob2@example.com")

	_, _ = testStore.CreateBookmark(ctx, alice.ID, "https://a1.com", "")
	_, _ = testStore.CreateBookmark(ctx, alice.ID, "https://a2.com", "")
	_, _ = testStore.CreateBookmark(ctx, bob.ID, "https://b1.com", "")

	got, err := testStore.ListBookmarks(ctx, alice.ID, 10, 0, uuid.Nil)
	if err != nil {
		t.Fatalf("ListBookmarks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (alice only)", len(got))
	}
}

func TestListBookmarks_Pagination(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "bm4@example.com")

	for i := 0; i < 5; i++ {
		_, _ = testStore.CreateBookmark(ctx, user.ID, "https://a.com", "")
	}

	page1, _ := testStore.ListBookmarks(ctx, user.ID, 2, 0, uuid.Nil)
	page2, _ := testStore.ListBookmarks(ctx, user.ID, 2, 2, uuid.Nil)
	if len(page1) != 2 || len(page2) != 2 {
		t.Fatalf("pagination wrong: page1=%d page2=%d", len(page1), len(page2))
	}
	if page1[0].ID == page2[0].ID {
		t.Fatal("pages overlap")
	}
}

func TestUpdateBookmark_Success(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "bm5@example.com")
	created, _ := testStore.CreateBookmark(ctx, user.ID, "https://old.com", "Old")

	updated, err := testStore.UpdateBookmark(ctx, user.ID, created.ID, "https://new.com", "New")
	if err != nil {
		t.Fatalf("UpdateBookmark: %v", err)
	}
	if updated.URL != "https://new.com" || updated.Title != "New" {
		t.Fatalf("update did not apply: %+v", updated)
	}
}

func TestUpdateBookmark_IDOR(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	alice := mustCreateUser(t, "alice3@example.com")
	bob := mustCreateUser(t, "bob3@example.com")
	aliceBookmark, _ := testStore.CreateBookmark(ctx, alice.ID, "https://a.com", "A")

	_, err := testStore.UpdateBookmark(ctx, bob.ID, aliceBookmark.ID, "https://hacked.com", "Hacked")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-user update, got %v", err)
	}
}

func TestDeleteBookmark_Success(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "bm6@example.com")
	created, _ := testStore.CreateBookmark(ctx, user.ID, "https://a.com", "")

	if err := testStore.DeleteBookmark(ctx, user.ID, created.ID); err != nil {
		t.Fatalf("DeleteBookmark: %v", err)
	}
	if _, err := testStore.GetBookmarkByID(ctx, user.ID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeleteBookmark_IDOR(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	alice := mustCreateUser(t, "alice4@example.com")
	bob := mustCreateUser(t, "bob4@example.com")
	aliceBookmark, _ := testStore.CreateBookmark(ctx, alice.ID, "https://a.com", "")

	err := testStore.DeleteBookmark(ctx, bob.ID, aliceBookmark.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-user delete, got %v", err)
	}
	// Alice's bookmark must still exist.
	if _, err := testStore.GetBookmarkByID(ctx, alice.ID, aliceBookmark.ID); err != nil {
		t.Fatalf("alice's bookmark was deleted: %v", err)
	}
}