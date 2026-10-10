package storage

import (
	"context"
	"fmt"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/google/uuid"
)

// AttachTag links a bookmark to a tag. Both must belong to the same user.
// Attaching an already-attached pair is a no-op (idempotent).
// Returns ErrNotFound if either resource is not owned by the user.
func (s *Store) AttachTag(ctx context.Context, userID, bookmarkID, tagID uuid.UUID) error {
	const ownCheck = `
		SELECT
			EXISTS(SELECT 1 FROM bookmarks WHERE id = $1 AND user_id = $3) AND
			EXISTS(SELECT 1 FROM tags      WHERE id = $2 AND user_id = $3)
	`
	var owned bool
	if err := s.db.QueryRowContext(ctx, ownCheck, bookmarkID, tagID, userID).Scan(&owned); err != nil {
		return fmt.Errorf("check ownership: %w", err)
	}
	if !owned {
		return ErrNotFound
	}

	const ins = `
		INSERT INTO bookmark_tags (bookmark_id, tag_id)
		VALUES ($1, $2)
		ON CONFLICT (bookmark_id, tag_id) DO NOTHING
	`
	if _, err := s.db.ExecContext(ctx, ins, bookmarkID, tagID); err != nil {
		return fmt.Errorf("attach tag: %w", err)
	}
	return nil
}

// DetachTag removes the link between a bookmark and a tag.
// Both must belong to the same user. Detaching a non-existent link is a
// no-op (idempotent). Returns ErrNotFound if either resource is not owned.
func (s *Store) DetachTag(ctx context.Context, userID, bookmarkID, tagID uuid.UUID) error {
	const ownCheck = `
		SELECT
			EXISTS(SELECT 1 FROM bookmarks WHERE id = $1 AND user_id = $3) AND
			EXISTS(SELECT 1 FROM tags      WHERE id = $2 AND user_id = $3)
	`
	var owned bool
	if err := s.db.QueryRowContext(ctx, ownCheck, bookmarkID, tagID, userID).Scan(&owned); err != nil {
		return fmt.Errorf("check ownership: %w", err)
	}
	if !owned {
		return ErrNotFound
	}

	const del = `DELETE FROM bookmark_tags WHERE bookmark_id = $1 AND tag_id = $2`
	if _, err := s.db.ExecContext(ctx, del, bookmarkID, tagID); err != nil {
		return fmt.Errorf("detach tag: %w", err)
	}
	return nil
}

// ListBookmarkTags returns all tags attached to a bookmark. The bookmark
// and the tags must belong to the user.
func (s *Store) ListBookmarkTags(ctx context.Context, userID, bookmarkID uuid.UUID) ([]domain.Tag, error) {
	const q = `
		SELECT t.id, t.user_id, t.name, t.created_at
		FROM tags t
		JOIN bookmark_tags bt ON bt.tag_id = t.id
		JOIN bookmarks b ON b.id = bt.bookmark_id
		WHERE bt.bookmark_id = $1 AND b.user_id = $2 AND t.user_id = $2
		ORDER BY t.name ASC
	`
	rows, err := s.db.QueryContext(ctx, q, bookmarkID, userID)
	if err != nil {
		return nil, fmt.Errorf("list bookmark tags: %w", err)
	}
	defer rows.Close()

	var out []domain.Tag
	for rows.Next() {
		var tag domain.Tag
		if err := rows.Scan(&tag.ID, &tag.UserID, &tag.Name, &tag.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}