package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// PasswordReset represents a password reset token.
type PasswordReset struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Token     string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// CreatePasswordReset inserts a new password reset token.
func (s *Store) CreatePasswordReset(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) (PasswordReset, error) {
	const q = `
		INSERT INTO password_resets (user_id, token, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token, expires_at, used_at, created_at
	`
	var pr PasswordReset
	err := s.db.QueryRowContext(ctx, q, userID, token, expiresAt).Scan(
		&pr.ID, &pr.UserID, &pr.Token, &pr.ExpiresAt, &pr.UsedAt, &pr.CreatedAt,
	)
	if err != nil {
		return PasswordReset{}, fmt.Errorf("create password reset: %w", err)
	}
	return pr, nil
}

// GetPasswordResetByToken returns the password reset token if it's valid (not expired, not used).
func (s *Store) GetPasswordResetByToken(ctx context.Context, token string) (PasswordReset, error) {
	// DEBUG: Log the token being queried
	slog.Info("DEBUG: GetPasswordResetByToken called", "token", token, "token_length", len(token))

	const q = `
		SELECT id, user_id, token, expires_at, used_at, created_at
		FROM password_resets
		WHERE token = $1
		  AND expires_at > NOW()
		  AND used_at IS NULL
	`
	var pr PasswordReset
	err := s.db.QueryRowContext(ctx, q, token).Scan(
		&pr.ID, &pr.UserID, &pr.Token, &pr.ExpiresAt, &pr.UsedAt, &pr.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.Info("DEBUG: Token not found in database (or expired/used)", "token", token)
			return PasswordReset{}, ErrNotFound
		}
		slog.Error("DEBUG: Database error in GetPasswordResetByToken", "err", err)
		return PasswordReset{}, fmt.Errorf("get password reset by token: %w", err)
	}
	slog.Info("DEBUG: Token found and valid", "token", token, "user_id", pr.UserID, "expires_at", pr.ExpiresAt, "used_at", pr.UsedAt)
	return pr, nil
}

// MarkPasswordResetUsed marks a password reset token as used.
func (s *Store) MarkPasswordResetUsed(ctx context.Context, id uuid.UUID) error {
	const q = `
		UPDATE password_resets
		SET used_at = NOW()
		WHERE id = $1
	`
	result, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mark password reset used: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateUserPassword updates the user's password hash.
func (s *Store) UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	const q = `
		UPDATE users
		SET password_hash = $1, updated_at = NOW()
		WHERE id = $2
	`
	result, err := s.db.ExecContext(ctx, q, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("update user password: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}