package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/google/uuid"
)

// CreateBookmark inserts a new bookmark for the given user.
func (s *Store) CreateBookmark(ctx context.Context, userID uuid.UUID, url, title string) (domain.Bookmark, error) {
	const q = `
		INSERT INTO bookmarks (user_id, url, title)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, url, title, created_at, updated_at
	`
	var b domain.Bookmark
	err := s.db.QueryRowContext(ctx, q, userID, url, title).Scan(
		&b.ID, &b.UserID, &b.URL, &b.Title, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return domain.Bookmark{}, fmt.Errorf("create bookmark: %w", err)
	}
	return b, nil
}

// GetBookmarkByID returns the bookmark with the given ID, scoped to the
// user. Returns ErrNotFound if the row does not exist or belongs to
// another user — the two cases are intentionally indistinguishable.
func (s *Store) GetBookmarkByID(ctx context.Context, userID, id uuid.UUID) (domain.Bookmark, error) {
	const q = `
		SELECT id, user_id, url, title, created_at, updated_at
		FROM bookmarks
		WHERE id = $1 AND user_id = $2
	`
	var b domain.Bookmark
	err := s.db.QueryRowContext(ctx, q, id, userID).Scan(
		&b.ID, &b.UserID, &b.URL, &b.Title, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Bookmark{}, ErrNotFound
		}
		return domain.Bookmark{}, fmt.Errorf("get bookmark: %w", err)
	}
	return b, nil
}

// ListBookmarks returns the user's bookmarks ordered by created_at DESC.
// limit and offset implement pagination. When tagID is not uuid.Nil the
// result is filtered to bookmarks that carry that tag.
func (s *Store) ListBookmarks(ctx context.Context, userID uuid.UUID, limit, offset int, tagID uuid.UUID) ([]domain.Bookmark, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if tagID == uuid.Nil {
		const q = `
			SELECT id, user_id, url, title, created_at, updated_at
			FROM bookmarks
			WHERE user_id = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
		rows, err = s.db.QueryContext(ctx, q, userID, limit, offset)
	} else {
		const q = `
			SELECT b.id, b.user_id, b.url, b.title, b.created_at, b.updated_at
			FROM bookmarks b
			JOIN bookmark_tags bt ON bt.bookmark_id = b.id
			WHERE b.user_id = $1 AND bt.tag_id = $2
			ORDER BY b.created_at DESC
			LIMIT $3 OFFSET $4
		`
		rows, err = s.db.QueryContext(ctx, q, userID, tagID, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list bookmarks: %w", err)
	}
	defer rows.Close()

	var out []domain.Bookmark
	for rows.Next() {
		var b domain.Bookmark
		if err := rows.Scan(&b.ID, &b.UserID, &b.URL, &b.Title, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan bookmark: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// UpdateBookmark updates url and title for a bookmark owned by the user.
// Returns ErrNotFound if no matching row was updated.
func (s *Store) UpdateBookmark(ctx context.Context, userID, id uuid.UUID, url, title string) (domain.Bookmark, error) {
	const q = `
		UPDATE bookmarks
		SET url = $1, title = $2, updated_at = NOW()
		WHERE id = $3 AND user_id = $4
		RETURNING id, user_id, url, title, created_at, updated_at
	`
	var b domain.Bookmark
	err := s.db.QueryRowContext(ctx, q, url, title, id, userID).Scan(
		&b.ID, &b.UserID, &b.URL, &b.Title, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Bookmark{}, ErrNotFound
		}
		return domain.Bookmark{}, fmt.Errorf("update bookmark: %w", err)
	}
	return b, nil
}

// DeleteBookmark removes a bookmark owned by the user.
// Returns ErrNotFound if no matching row was deleted.
func (s *Store) DeleteBookmark(ctx context.Context, userID, id uuid.UUID) error {
	const q = `DELETE FROM bookmarks WHERE id = $1 AND user_id = $2`
	res, err := s.db.ExecContext(ctx, q, id, userID)
	if err != nil {
		return fmt.Errorf("delete bookmark: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete bookmark rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}