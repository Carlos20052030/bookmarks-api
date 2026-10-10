package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/google/uuid"
)

// ErrTagNameTaken is returned when a tag with the same (user_id, name)
// pair already exists. Mirrors ErrEmailTaken for users.
var ErrTagNameTaken = errors.New("tag name already taken")

// CreateTag inserts a new tag for the user. Returns ErrTagNameTaken when
// the user already has a tag with the same name.
func (s *Store) CreateTag(ctx context.Context, userID uuid.UUID, name string) (domain.Tag, error) {
	const q = `
		INSERT INTO tags (user_id, name)
		VALUES ($1, $2)
		RETURNING id, user_id, name, created_at
	`
	var tag domain.Tag
	err := s.db.QueryRowContext(ctx, q, userID, name).Scan(
		&tag.ID, &tag.UserID, &tag.Name, &tag.CreatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.Tag{}, ErrTagNameTaken
		}
		return domain.Tag{}, fmt.Errorf("create tag: %w", err)
	}
	return tag, nil
}

// ListTags returns all tags owned by the user, ordered by name.
func (s *Store) ListTags(ctx context.Context, userID uuid.UUID) ([]domain.Tag, error) {
	const q = `
		SELECT id, user_id, name, created_at
		FROM tags
		WHERE user_id = $1
		ORDER BY name ASC
	`
	rows, err := s.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
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

// GetTagByID returns the tag with the given ID, scoped to the user.
// Returns ErrNotFound when the tag does not exist or belongs to another user.
func (s *Store) GetTagByID(ctx context.Context, userID, id uuid.UUID) (domain.Tag, error) {
	const q = `
		SELECT id, user_id, name, created_at
		FROM tags
		WHERE id = $1 AND user_id = $2
	`
	var tag domain.Tag
	err := s.db.QueryRowContext(ctx, q, id, userID).Scan(
		&tag.ID, &tag.UserID, &tag.Name, &tag.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Tag{}, ErrNotFound
		}
		return domain.Tag{}, fmt.Errorf("get tag: %w", err)
	}
	return tag, nil
}

// DeleteTag removes a tag owned by the user.
// Returns ErrNotFound if no matching row was deleted.
func (s *Store) DeleteTag(ctx context.Context, userID, id uuid.UUID) error {
	const q = `DELETE FROM tags WHERE id = $1 AND user_id = $2`
	res, err := s.db.ExecContext(ctx, q, id, userID)
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete tag rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}