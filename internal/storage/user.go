package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Carlos20052030/bookmarks-api/internal/billing/domain"
	mainDomain "github.com/Carlos20052030/bookmarks-api/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

// CreateUser inserts a new user and returns the stored row.
// It returns ErrEmailTaken if the email already exists.
func (s *Store) CreateUser(ctx context.Context, email, passwordHash string) (mainDomain.User, error) {
	const q = `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, password_hash, created_at, updated_at
	`
	var u mainDomain.User
	err := s.db.QueryRowContext(ctx, q, email, passwordHash).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return mainDomain.User{}, ErrEmailTaken
		}
		return mainDomain.User{}, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

// GetUserByEmail returns the user with the given email.
// It returns ErrNotFound if no row matches.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (mainDomain.User, error) {
	const q = `
		SELECT id, email, password_hash, created_at, updated_at
		FROM users
		WHERE email = $1
	`
	var u mainDomain.User
	err := s.db.QueryRowContext(ctx, q, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mainDomain.User{}, ErrNotFound
		}
		return mainDomain.User{}, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// GetUserBillingInfo returns the billing information for a user.
// Returns plan, subscription_status, current_period_end, webhook_count, and error.
func (s *Store) GetUserBillingInfo(ctx context.Context, userID string) (domain.Plan, domain.SubscriptionStatus, sql.NullTime, int, error) {
	const q = `
		SELECT plan, subscription_status, current_period_end
		FROM users
		WHERE id = $1
	`
	var planStr, statusStr string
	var periodEnd sql.NullTime
	err := s.db.QueryRowContext(ctx, q, userID).Scan(&planStr, &statusStr, &periodEnd)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.PlanFree, domain.SubscriptionStatusNone, sql.NullTime{}, 0, ErrNotFound
		}
		return domain.PlanFree, domain.SubscriptionStatusNone, sql.NullTime{}, 0, fmt.Errorf("get user billing info: %w", err)
	}

	plan := domain.ParsePlanOrDefault(planStr)
	status := domain.SubscriptionStatusNone
	switch statusStr {
	case "incomplete":
		status = domain.SubscriptionStatusIncomplete
	case "active":
		status = domain.SubscriptionStatusActive
	case "past_due":
		status = domain.SubscriptionStatusPastDue
	case "canceled":
		status = domain.SubscriptionStatusCanceled
	default:
		status = domain.SubscriptionStatusNone
	}

	// TODO: webhook_count should come from a webhooks table or quota service
	// For now, return 0 since there's no webhooks table
	webhookCount := 0

	return plan, status, periodEnd, webhookCount, nil
}
