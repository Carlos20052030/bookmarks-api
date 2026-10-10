package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestCreateTag_Success(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "tag1@example.com")

	tag, err := testStore.CreateTag(ctx, user.ID, "go")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if tag.ID == uuid.Nil {
		t.Fatal("tag ID is nil")
	}
	if tag.Name != "go" {
		t.Fatalf("Name = %q, want go", tag.Name)
	}
}

func TestCreateTag_DuplicateNameForSameUser(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "tag2@example.com")

	if _, err := testStore.CreateTag(ctx, user.ID, "go"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := testStore.CreateTag(ctx, user.ID, "go")
	if !errors.Is(err, ErrTagNameTaken) {
		t.Fatalf("expected ErrTagNameTaken, got %v", err)
	}
}

func TestCreateTag_SameNameDifferentUsersAllowed(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	alice := mustCreateUser(t, "tag3@example.com")
	bob := mustCreateUser(t, "tag4@example.com")

	if _, err := testStore.CreateTag(ctx, alice.ID, "go"); err != nil {
		t.Fatalf("alice: %v", err)
	}
	if _, err := testStore.CreateTag(ctx, bob.ID, "go"); err != nil {
		t.Fatalf("bob should be allowed: %v", err)
	}
}

func TestListTags_OnlyOwn(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	alice := mustCreateUser(t, "tag5@example.com")
	bob := mustCreateUser(t, "tag6@example.com")

	_, _ = testStore.CreateTag(ctx, alice.ID, "go")
	_, _ = testStore.CreateTag(ctx, alice.ID, "rust")
	_, _ = testStore.CreateTag(ctx, bob.ID, "python")

	got, err := testStore.ListTags(ctx, alice.ID)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// Ordered by name ASC.
	if got[0].Name != "go" || got[1].Name != "rust" {
		t.Fatalf("order wrong: %s, %s", got[0].Name, got[1].Name)
	}
}

func TestGetTagByID_IDOR(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	alice := mustCreateUser(t, "tag7@example.com")
	bob := mustCreateUser(t, "tag8@example.com")

	aliceTag, _ := testStore.CreateTag(ctx, alice.ID, "secret")

	_, err := testStore.GetTagByID(ctx, bob.ID, aliceTag.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-user access, got %v", err)
	}
}

func TestDeleteTag_IDOR(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	alice := mustCreateUser(t, "tag9@example.com")
	bob := mustCreateUser(t, "tag10@example.com")

	aliceTag, _ := testStore.CreateTag(ctx, alice.ID, "keep")

	err := testStore.DeleteTag(ctx, bob.ID, aliceTag.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-user delete, got %v", err)
	}
	// Alice's tag must still exist.
	if _, err := testStore.GetTagByID(ctx, alice.ID, aliceTag.ID); err != nil {
		t.Fatalf("alice's tag was deleted: %v", err)
	}
}

func TestDeleteTag_Success(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	user := mustCreateUser(t, "tag11@example.com")

	tag, _ := testStore.CreateTag(ctx, user.ID, "gone")
	if err := testStore.DeleteTag(ctx, user.ID, tag.ID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if _, err := testStore.GetTagByID(ctx, user.ID, tag.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}