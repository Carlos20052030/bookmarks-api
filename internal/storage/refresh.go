package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/google/uuid"
)

// CreateRefreshToken stores a new refresh token hash for a user.
func (s *Store) CreateRefreshToken(
	ctx context.Context,
	userID uuid.UUID,
	tokenHash string,
	expiresAt time.Time,
) (domain.RefreshToken, error) {
	const q = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token_hash, expires_at, revoked_at, created_at
	`
	var t domain.RefreshToken
	err := s.db.QueryRowContext(ctx, q, userID, tokenHash, expiresAt).Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt,
	)
	if err != nil {
		return domain.RefreshToken{}, fmt.Errorf("create refresh token: %w", err)
	}
	return t, nil
}

// GetRefreshTokenByHash returns the token row matching the given hash.
// It returns ErrNotFound if no row matches.
func (s *Store) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (domain.RefreshToken, error) {
	const q = `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`
	var t domain.RefreshToken
	err := s.db.QueryRowContext(ctx, q, tokenHash).Scan(
		&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.RefreshToken{}, ErrNotFound
		}
		return domain.RefreshToken{}, fmt.Errorf("get refresh token: %w", err)
	}
	return t, nil
}

// RevokeRefreshToken marks the token as revoked with the current timestamp.
// Revoking an already-revoked token is a no-op, not an error.
func (s *Store) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	const q = `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE id = $1 AND revoked_at IS NULL
	`
	if _, err := s.db.ExecContext(ctx, q, id); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}
