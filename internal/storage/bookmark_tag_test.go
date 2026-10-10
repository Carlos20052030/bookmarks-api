package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestAttachTag_Success(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "at1@example.com")
	bm, _ := testStore.CreateBookmark(ctx, user.ID, "https://a.com", "")
	tag, _ := testStore.CreateTag(ctx, user.ID, "go")

	if err := testStore.AttachTag(ctx, user.ID, bm.ID, tag.ID); err != nil {
		t.Fatalf("AttachTag: %v", err)
	}

	tags, err := testStore.ListBookmarkTags(ctx, user.ID, bm.ID)
	if err != nil {
		t.Fatalf("ListBookmarkTags: %v", err)
	}
	if len(tags) != 1 || tags[0].ID != tag.ID {
		t.Fatalf("tag not attached: %+v", tags)
	}
}

func TestAttachTag_Idempotent(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "at2@example.com")
	bm, _ := testStore.CreateBookmark(ctx, user.ID, "https://a.com", "")
	tag, _ := testStore.CreateTag(ctx, user.ID, "go")

	if err := testStore.AttachTag(ctx, user.ID, bm.ID, tag.ID); err != nil {
		t.Fatalf("first attach: %v", err)
	}
	if err := testStore.AttachTag(ctx, user.ID, bm.ID, tag.ID); err != nil {
		t.Fatalf("second attach should be no-op, got: %v", err)
	}
}

func TestAttachTag_NotOwnedBookmark(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	alice := mustCreateUser(t, "at3@example.com")
	bob := mustCreateUser(t, "at4@example.com")

	aliceBookmark, _ := testStore.CreateBookmark(ctx, alice.ID, "https://a.com", "")
	bobTag, _ := testStore.CreateTag(ctx, bob.ID, "go")

	err := testStore.AttachTag(ctx, bob.ID, aliceBookmark.ID, bobTag.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-user bookmark, got %v", err)
	}
}

func TestAttachTag_NotOwnedTag(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	alice := mustCreateUser(t, "at5@example.com")
	bob := mustCreateUser(t, "at6@example.com")

	bobBookmark, _ := testStore.CreateBookmark(ctx, bob.ID, "https://b.com", "")
	aliceTag, _ := testStore.CreateTag(ctx, alice.ID, "secret")

	err := testStore.AttachTag(ctx, bob.ID, bobBookmark.ID, aliceTag.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-user tag, got %v", err)
	}
}

func TestDetachTag_Success(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "dt1@example.com")
	bm, _ := testStore.CreateBookmark(ctx, user.ID, "https://a.com", "")
	tag, _ := testStore.CreateTag(ctx, user.ID, "go")

	_ = testStore.AttachTag(ctx, user.ID, bm.ID, tag.ID)
	if err := testStore.DetachTag(ctx, user.ID, bm.ID, tag.ID); err != nil {
		t.Fatalf("DetachTag: %v", err)
	}

	tags, _ := testStore.ListBookmarkTags(ctx, user.ID, bm.ID)
	if len(tags) != 0 {
		t.Fatalf("tag still attached: %+v", tags)
	}
}

func TestDetachTag_Idempotent(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "dt2@example.com")
	bm, _ := testStore.CreateBookmark(ctx, user.ID, "https://a.com", "")
	tag, _ := testStore.CreateTag(ctx, user.ID, "go")

	_ = testStore.AttachTag(ctx, user.ID, bm.ID, tag.ID)
	if err := testStore.DetachTag(ctx, user.ID, bm.ID, tag.ID); err != nil {
		t.Fatalf("first detach: %v", err)
	}
	if err := testStore.DetachTag(ctx, user.ID, bm.ID, tag.ID); err != nil {
		t.Fatalf("second detach should be no-op, got: %v", err)
	}
}

func TestListBookmarkTags_OnlyOwn(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	alice := mustCreateUser(t, "lbt1@example.com")
	bob := mustCreateUser(t, "lbt2@example.com")

	aliceBookmark, _ := testStore.CreateBookmark(ctx, alice.ID, "https://a.com", "")
	aliceTag, _ := testStore.CreateTag(ctx, alice.ID, "go")
	_ = testStore.AttachTag(ctx, alice.ID, aliceBookmark.ID, aliceTag.ID)

	// Bob tries to see tags of Alice's bookmark using her ID.
	tags, err := testStore.ListBookmarkTags(ctx, bob.ID, aliceBookmark.ID)
	if err != nil {
		t.Fatalf("ListBookmarkTags: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("bob should see 0 tags, got %d", len(tags))
	}
}

func TestListBookmarks_TagFilter(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "tf1@example.com")

	bm1, _ := testStore.CreateBookmark(ctx, user.ID, "https://a.com", "")
	bm2, _ := testStore.CreateBookmark(ctx, user.ID, "https://b.com", "")
	_, _ = testStore.CreateBookmark(ctx, user.ID, "https://c.com", "")

	tag, _ := testStore.CreateTag(ctx, user.ID, "go")
	_ = testStore.AttachTag(ctx, user.ID, bm1.ID, tag.ID)
	_ = testStore.AttachTag(ctx, user.ID, bm2.ID, tag.ID)

	got, err := testStore.ListBookmarks(ctx, user.ID, 10, 0, tag.ID)
	if err != nil {
		t.Fatalf("ListBookmarks with tag: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (tagged)", len(got))
	}
}

var _ = uuid.Nil