package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/google/uuid"
)

// CreateAPIKey inserts a new key into the database.
func (s *Store) CreateAPIKey(ctx context.Context, userID uuid.UUID, name, keyPrefix, keyHash string) (domain.APIKey, error) {
	const q = `
		INSERT INTO api_keys (user_id, name, key_prefix, key_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, key_prefix, key_hash, created_at, revoked_at
	`
	var k domain.APIKey
	err := s.db.QueryRowContext(ctx, q, userID, name, keyPrefix, keyHash).Scan(
		&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.CreatedAt, &k.RevokedAt,
	)
	if err != nil {
		return domain.APIKey{}, fmt.Errorf("create api key: %w", err)
	}
	return k, nil
}

// GetAPIKeyByHash retrieves an active key by its hash (used for authentication).
// Note: With bcrypt, this method is not used for authentication anymore.
// Kept for backwards compatibility if needed.
func (s *Store) GetAPIKeyByHash(ctx context.Context, hash string) (domain.APIKey, error) {
	const q = `
		SELECT id, user_id, name, key_prefix, key_hash, created_at, revoked_at
		FROM api_keys
		WHERE key_hash = $1 AND revoked_at IS NULL
	`
	var k domain.APIKey
	err := s.db.QueryRowContext(ctx, q, hash).Scan(
		&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.CreatedAt, &k.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.APIKey{}, errors.New("api key not found or revoked")
		}
		return domain.APIKey{}, fmt.Errorf("get api key: %w", err)
	}
	return k, nil
}

// GetAPIKeysByPrefix retrieves all active keys matching the given prefix.
// Used for bcrypt comparison since we can't do exact hash matching.
func (s *Store) GetAPIKeysByPrefix(ctx context.Context, prefix string) ([]domain.APIKey, error) {
	const q = `
		SELECT id, user_id, name, key_prefix, key_hash, created_at, revoked_at
		FROM api_keys
		WHERE key_prefix = $1 AND revoked_at IS NULL
	`
	rows, err := s.db.QueryContext(ctx, q, prefix)
	if err != nil {
		return nil, fmt.Errorf("get api keys by prefix: %w", err)
	}
	defer rows.Close()

	var out []domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// ListAPIKeys returns all keys for a specific user.
func (s *Store) ListAPIKeys(ctx context.Context, userID uuid.UUID) ([]domain.APIKey, error) {
	const q = `
		SELECT id, user_id, name, key_prefix, key_hash, created_at, revoked_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := s.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var out []domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeAPIKey marks a key as revoked.
func (s *Store) RevokeAPIKey(ctx context.Context, userID, id uuid.UUID) error {
	const q = `
		UPDATE api_keys
		SET revoked_at = NOW()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL
	`
	res, err := s.db.ExecContext(ctx, q, id, userID)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("api key not found or already revoked")
	}
	return nil
}
