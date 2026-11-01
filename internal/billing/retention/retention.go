package retention

import (
	"log/slog"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/billing/domain"
)

// DBInterface defines the database operations needed for retention
type DBInterface interface {
	GetOldDeliveryAttempts(plan domain.Plan, olderThan time.Time) ([]DeliveryAttempt, error)
	DeleteDeliveryAttempts(ids []int64) (int64, error)
}

// DeliveryAttempt represents a webhook delivery attempt
type DeliveryAttempt struct {
	ID        int64
	UserID    string
	Plan      domain.Plan
	CreatedAt time.Time
}

// CleanupResult contains the result of a cleanup operation
type CleanupResult struct {
	DeletedFree int64
	DeletedPro  int64
	TotalDeleted int64
}

// RetentionService handles data retention cleanup
type RetentionService struct {
	db     DBInterface
	logger *slog.Logger
}

func NewRetentionService(db DBInterface) *RetentionService {
	return &RetentionService{
		db:     db,
		logger: slog.Default().With("component", "retention"),
	}
}

// CutoffFor returns the cutoff time for a given plan
func (s *RetentionService) CutoffFor(plan domain.Plan, now time.Time) time.Time {
	quotas := domain.QuotasFor(plan)
	return now.AddDate(0, 0, -quotas.RetentionDays)
}

// Cleanup deletes old delivery attempts for a specific plan
func (s *RetentionService) Cleanup(plan domain.Plan, cutoff time.Time) (int64, error) {
	attempts, err := s.db.GetOldDeliveryAttempts(plan, cutoff)
	if err != nil {
		return 0, err
	}

	if len(attempts) == 0 {
		return 0, nil
	}

	ids := make([]int64, len(attempts))
	for i, a := range attempts {
		ids[i] = a.ID
	}

	deleted, err := s.db.DeleteDeliveryAttempts(ids)
	if err != nil {
		return 0, err
	}

	s.logger.Info("cleanup completed", "plan", plan, "deleted", deleted, "cutoff", cutoff)
	return deleted, nil
}

// CleanupAll runs cleanup for all plans
func (s *RetentionService) CleanupAll(now time.Time) (*CleanupResult, error) {
	result := &CleanupResult{}

	// Clean Free plan
	freeCutoff := s.CutoffFor(domain.PlanFree, now)
	deletedFree, err := s.Cleanup(domain.PlanFree, freeCutoff)
	if err != nil {
		return nil, err
	}
	result.DeletedFree = deletedFree

	// Clean Pro plan
	proCutoff := s.CutoffFor(domain.PlanPro, now)
	deletedPro, err := s.Cleanup(domain.PlanPro, proCutoff)
	if err != nil {
		return nil, err
	}
	result.DeletedPro = deletedPro

	result.TotalDeleted = deletedFree + deletedPro
	return result, nil
}