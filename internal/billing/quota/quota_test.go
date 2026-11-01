package quota

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/billing/domain"
)

// TestQuotaServiceInterface ensures the interface is properly defined
func TestQuotaServiceInterface(t *testing.T) {
	var _ QuotaService = (*InMemoryQuotaService)(nil)
}

func TestInMemoryQuotaService_CanCreateWebhook_FreePlan(t *testing.T) {
	svc := NewInMemoryQuotaService()
	userID := "user-free-1"

	// Free plan: max 3 webhooks
	for i := 0; i < 3; i++ {
		allowed, err := svc.CanCreateWebhook(userID, domain.PlanFree, i)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("expected allowed for webhook %d, got denied", i+1)
		}
	}

	// 4th webhook should be denied
	allowed, err := svc.CanCreateWebhook(userID, domain.PlanFree, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("expected denied for 4th webhook, got allowed")
	}
}

func TestInMemoryQuotaService_CanCreateWebhook_ProPlan(t *testing.T) {
	svc := NewInMemoryQuotaService()
	userID := "user-pro-1"

	// Pro plan: max 50 webhooks
	for i := 0; i < 50; i++ {
		allowed, err := svc.CanCreateWebhook(userID, domain.PlanPro, i)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("expected allowed for webhook %d, got denied", i+1)
		}
	}

	// 51st webhook should be denied
	allowed, err := svc.CanCreateWebhook(userID, domain.PlanPro, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("expected denied for 51st webhook, got allowed")
	}
}

func TestInMemoryQuotaService_CanDeliverRequest_FreePlan(t *testing.T) {
	svc := NewInMemoryQuotaService()
	userID := "user-free-delivery"

	// Free plan: max 10,000 requests
	for i := 0; i < 10_000; i++ {
		allowed, err := svc.CanDeliverRequest(userID, domain.PlanFree, i)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("expected allowed for request %d, got denied", i+1)
		}
	}

	// 10,001st request should be denied
	allowed, err := svc.CanDeliverRequest(userID, domain.PlanFree, 10_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("expected denied for 10,001st request, got allowed")
	}
}

func TestInMemoryQuotaService_CanDeliverRequest_ProPlan(t *testing.T) {
	svc := NewInMemoryQuotaService()
	userID := "user-pro-delivery"

	// Pro plan: max 1,000,000 requests
	for i := 0; i < 1_000_000; i++ {
		allowed, err := svc.CanDeliverRequest(userID, domain.PlanPro, i)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("expected allowed for request %d, got denied", i+1)
		}
	}

	// 1,000,001st request should be denied
	allowed, err := svc.CanDeliverRequest(userID, domain.PlanPro, 1_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("expected denied for 1,000,001st request, got allowed")
	}
}

func TestInMemoryQuotaService_Concurrency_CanCreateWebhook(t *testing.T) {
	svc := NewInMemoryQuotaService()
	userID := "user-concurrent-webhook"
	const goroutines = 100
	const limit = 3 // Free plan limit

	var allowedCount int64
	var deniedCount int64
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine tries to create a webhook
			// Use atomic to get current count
			current := atomic.LoadInt64(&allowedCount) + atomic.LoadInt64(&deniedCount)
			allowed, err := svc.CanCreateWebhook(userID, domain.PlanFree, int(current))
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if allowed {
				atomic.AddInt64(&allowedCount, 1)
			} else {
				atomic.AddInt64(&deniedCount, 1)
			}
		}()
	}

	wg.Wait()

	// Exactly 3 should be allowed, rest denied
	if allowedCount != limit {
		t.Errorf("expected %d allowed, got %d", limit, allowedCount)
	}
	if deniedCount != goroutines-limit {
		t.Errorf("expected %d denied, got %d", goroutines-limit, deniedCount)
	}
}

func TestInMemoryQuotaService_Concurrency_CanDeliverRequest(t *testing.T) {
	svc := NewInMemoryQuotaService()
	userID := "user-concurrent-delivery"

	// Free plan request limit is 10,000
	planQuotas := domain.QuotasFor(domain.PlanFree)
	const goroutines = 12000 // Exceed Free plan limit
	limit := planQuotas.MaxRequestsPerPeriod

	var allowedCount int64
	var deniedCount int64
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// currentCount parameter is ignored by implementation (uses internal counter)
			allowed, err := svc.CanDeliverRequest(userID, domain.PlanFree, 0)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if allowed {
				atomic.AddInt64(&allowedCount, 1)
			} else {
				atomic.AddInt64(&deniedCount, 1)
			}
		}()
	}

	wg.Wait()

	// Exactly 'limit' should be allowed, rest denied
	if allowedCount != int64(limit) {
		t.Errorf("expected %d allowed, got %d", limit, allowedCount)
	}
	if deniedCount != int64(goroutines-limit) {
		t.Errorf("expected %d denied, got %d", goroutines-limit, deniedCount)
	}
}

func TestInMemoryQuotaService_MultipleUsers_Isolated(t *testing.T) {
	svc := NewInMemoryQuotaService()

	// User 1 creates 3 webhooks (Free limit)
	for i := 0; i < 3; i++ {
		allowed, err := svc.CanCreateWebhook("user-1", domain.PlanFree, i)
		if err != nil || !allowed {
			t.Fatalf("user-1 webhook %d: expected allowed, got %v, err: %v", i+1, allowed, err)
		}
	}

	// User 2 should also be able to create 3 webhooks independently
	for i := 0; i < 3; i++ {
		allowed, err := svc.CanCreateWebhook("user-2", domain.PlanFree, i)
		if err != nil || !allowed {
			t.Fatalf("user-2 webhook %d: expected allowed, got %v, err: %v", i+1, allowed, err)
		}
	}

	// User 1's 4th should be denied
	allowed, err := svc.CanCreateWebhook("user-1", domain.PlanFree, 3)
	if err != nil || allowed {
		t.Errorf("user-1 4th webhook: expected denied, got allowed=%v, err=%v", allowed, err)
	}

	// User 2's 4th should be denied
	allowed, err = svc.CanCreateWebhook("user-2", domain.PlanFree, 3)
	if err != nil || allowed {
		t.Errorf("user-2 4th webhook: expected denied, got allowed=%v, err=%v", allowed, err)
	}
}

func TestInMemoryQuotaService_ResetForNewPeriod(t *testing.T) {
	svc := NewInMemoryQuotaService()
	userID := "user-period-reset"

	// Use up Free plan requests
	for i := 0; i < 10_000; i++ {
		allowed, err := svc.CanDeliverRequest(userID, domain.PlanFree, i)
		if err != nil || !allowed {
			t.Fatalf("request %d: expected allowed, got %v, err: %v", i+1, allowed, err)
		}
	}

	// Next should be denied
	allowed, err := svc.CanDeliverRequest(userID, domain.PlanFree, 10_000)
	if err != nil || allowed {
		t.Errorf("expected denied, got allowed=%v, err=%v", allowed, err)
	}

	// Reset for new period
	svc.ResetUser(userID)

	// Should be allowed again
	for i := 0; i < 5; i++ {
		allowed, err := svc.CanDeliverRequest(userID, domain.PlanFree, i)
		if err != nil || !allowed {
			t.Fatalf("after reset request %d: expected allowed, got %v, err: %v", i+1, allowed, err)
		}
	}
}

func TestInMemoryQuotaService_GetCurrentUsage(t *testing.T) {
	svc := NewInMemoryQuotaService()
	userID := "user-usage"

	// Initially zero
	usage := svc.GetCurrentUsage(userID)
	if usage.WebhookCount != 0 || usage.RequestCount != 0 {
		t.Errorf("expected zero usage, got %+v", usage)
	}

	// Create 2 webhooks
	for i := 0; i < 2; i++ {
		_, _ = svc.CanCreateWebhook(userID, domain.PlanFree, i)
	}

	// Make 5 deliveries
	for i := 0; i < 5; i++ {
		_, _ = svc.CanDeliverRequest(userID, domain.PlanFree, i)
	}

	usage = svc.GetCurrentUsage(userID)
	if usage.WebhookCount != 2 {
		t.Errorf("expected webhook count 2, got %d", usage.WebhookCount)
	}
	if usage.RequestCount != 5 {
		t.Errorf("expected request count 5, got %d", usage.RequestCount)
	}
}

func TestInMemoryQuotaService_UsageStruct(t *testing.T) {
	usage := UserUsage{
		WebhookCount:  5,
		RequestCount:  100,
		LastUpdatedAt: time.Now(),
	}

	if usage.WebhookCount != 5 {
		t.Errorf("WebhookCount = %d, want 5", usage.WebhookCount)
	}
	if usage.RequestCount != 100 {
		t.Errorf("RequestCount = %d, want 100", usage.RequestCount)
	}
	if usage.LastUpdatedAt.IsZero() {
		t.Error("LastUpdatedAt should not be zero")
	}
}