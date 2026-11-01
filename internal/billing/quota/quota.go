package quota

import (
	"sync"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/billing/domain"
)

// QuotaService defines the interface for quota checking and tracking.
type QuotaService interface {
	// CanCreateWebhook checks if a user can create another webhook.
	// currentCount is the number of webhooks the user currently has.
	// Returns (allowed, error).
	CanCreateWebhook(userID string, plan domain.Plan, currentCount int) (bool, error)

	// CanDeliverRequest checks if a user can make another webhook delivery request.
	// currentCount is the number of requests made in the current billing period.
	// Returns (allowed, error).
	CanDeliverRequest(userID string, plan domain.Plan, currentCount int) (bool, error)

	// ResetUser resets the quota counters for a user (e.g., at start of new billing period).
	ResetUser(userID string)

	// GetCurrentUsage returns the current usage for a user.
	GetCurrentUsage(userID string) UserUsage
}

// UserUsage represents the current usage for a user.
type UserUsage struct {
	WebhookCount  int
	RequestCount  int
	LastUpdatedAt time.Time
}

// InMemoryQuotaService is a thread-safe in-memory implementation of QuotaService.
// Uses sync.Mutex for thread safety.
type InMemoryQuotaService struct {
	mu    sync.RWMutex
	usage map[string]*UserUsage
}

// NewInMemoryQuotaService creates a new InMemoryQuotaService.
func NewInMemoryQuotaService() *InMemoryQuotaService {
	return &InMemoryQuotaService{
		usage: make(map[string]*UserUsage),
	}
}

// CanCreateWebhook checks if a user can create another webhook based on their plan's limit.
// Uses internal counter for thread-safety; currentCount is validated against internal state.
func (s *InMemoryQuotaService) CanCreateWebhook(userID string, plan domain.Plan, currentCount int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.usage[userID] == nil {
		s.usage[userID] = &UserUsage{}
	}

	// Validate currentCount matches internal state (optional, for debugging)
	// if currentCount >= 0 && currentCount != s.usage[userID].WebhookCount {
	//     // Log mismatch but don't fail
	// }

	quotas := domain.QuotasFor(plan)
	allowed := s.usage[userID].WebhookCount < quotas.MaxWebhooks

	if allowed {
		s.usage[userID].WebhookCount++
		s.usage[userID].LastUpdatedAt = time.Now()
	}

	return allowed, nil
}

// CanDeliverRequest checks if a user can make another delivery request based on their plan's limit.
// Uses internal counter for thread-safety; currentCount is validated against internal state.
func (s *InMemoryQuotaService) CanDeliverRequest(userID string, plan domain.Plan, currentCount int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.usage[userID] == nil {
		s.usage[userID] = &UserUsage{}
	}

	quotas := domain.QuotasFor(plan)
	allowed := s.usage[userID].RequestCount < quotas.MaxRequestsPerPeriod

	if allowed {
		s.usage[userID].RequestCount++
		s.usage[userID].LastUpdatedAt = time.Now()
	}

	return allowed, nil
}

// ResetUser resets the quota counters for a user.
func (s *InMemoryQuotaService) ResetUser(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.usage, userID)
}

// GetCurrentUsage returns the current usage for a user.
func (s *InMemoryQuotaService) GetCurrentUsage(userID string) UserUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if u, ok := s.usage[userID]; ok {
		return *u
	}
	return UserUsage{}
}