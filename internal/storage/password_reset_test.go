package storage

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPasswordResetStore(t *testing.T) {
	ctx := context.Background()

	t.Run("CreatePasswordReset creates a reset token", func(t *testing.T) {
		cleanTables(t)
		user := mustCreateUser(t, "test-create-reset@example.com")

		token := "test-token-1234567890abcdef1234567890abcdef"
		expiresAt := time.Now().Add(1 * time.Hour)

		reset, err := testStore.CreatePasswordReset(ctx, user.ID, token, expiresAt)
		require.NoError(t, err)
		require.Equal(t, user.ID, reset.UserID)
		require.Equal(t, token, reset.Token)
		require.Equal(t, expiresAt.Truncate(time.Second), reset.ExpiresAt.Truncate(time.Second))
		require.Nil(t, reset.UsedAt)
		require.NotEqual(t, uuid.Nil, reset.ID)
	})

	t.Run("GetPasswordResetByToken returns the reset token", func(t *testing.T) {
		cleanTables(t)
		user := mustCreateUser(t, "test-get-by-token@example.com")
		token := "test-token-get-by-token"
		expiresAt := time.Now().Add(1 * time.Hour)

		created, err := testStore.CreatePasswordReset(ctx, user.ID, token, expiresAt)
		require.NoError(t, err)

		reset, err := testStore.GetPasswordResetByToken(ctx, token)
		require.NoError(t, err)
		require.Equal(t, created.ID, reset.ID)
		require.Equal(t, created.UserID, reset.UserID)
		require.Equal(t, created.Token, reset.Token)
		require.Equal(t, created.ExpiresAt, reset.ExpiresAt)
		require.Nil(t, reset.UsedAt)
	})

	t.Run("GetPasswordResetByToken returns ErrNotFound for non-existent token", func(t *testing.T) {
		cleanTables(t)
		_, err := testStore.GetPasswordResetByToken(ctx, "non-existent-token")
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("GetPasswordResetByToken returns ErrNotFound for expired token", func(t *testing.T) {
		cleanTables(t)
		user := mustCreateUser(t, "test-expired@example.com")
		token := "test-token-expired"
		expiresAt := time.Now().Add(-1 * time.Hour)

		_, err := testStore.CreatePasswordReset(ctx, user.ID, token, expiresAt)
		require.NoError(t, err)

		_, err = testStore.GetPasswordResetByToken(ctx, token)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("GetPasswordResetByToken returns ErrNotFound for used token", func(t *testing.T) {
		cleanTables(t)
		user := mustCreateUser(t, "test-used@example.com")
		token := "test-token-used"
		expiresAt := time.Now().Add(1 * time.Hour)

		_, err := testStore.CreatePasswordReset(ctx, user.ID, token, expiresAt)
		require.NoError(t, err)

		// Mark as used
		now := time.Now()
		_, err = testStore.db.ExecContext(ctx, `UPDATE password_resets SET used_at = $1 WHERE token = $2`, now, token)
		require.NoError(t, err)

		_, err = testStore.GetPasswordResetByToken(ctx, token)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("MarkPasswordResetUsed marks the token as used", func(t *testing.T) {
		cleanTables(t)
		user := mustCreateUser(t, "test-mark-used@example.com")
		token := "test-token-mark-used"
		expiresAt := time.Now().Add(1 * time.Hour)

		reset, err := testStore.CreatePasswordReset(ctx, user.ID, token, expiresAt)
		require.NoError(t, err)

		err = testStore.MarkPasswordResetUsed(ctx, reset.ID)
		require.NoError(t, err)

		// Verify it's marked as used
		var usedAt time.Time
		err = testStore.db.QueryRowContext(ctx, `SELECT used_at FROM password_resets WHERE id = $1`, reset.ID).Scan(&usedAt)
		require.NoError(t, err)
		require.False(t, usedAt.IsZero())
	})

	t.Run("UpdateUserPassword updates the user's password hash", func(t *testing.T) {
		cleanTables(t)
		user := mustCreateUser(t, "test-update-password@example.com")
		newHash := "$2a$12$newhashnewhashnewhashnewhashnew"

		err := testStore.UpdateUserPassword(ctx, user.ID, newHash)
		require.NoError(t, err)

		// Verify the password was updated
		updatedUser, err := testStore.GetUserByEmail(ctx, user.Email)
		require.NoError(t, err)
		require.Equal(t, newHash, updatedUser.PasswordHash)
	})
}