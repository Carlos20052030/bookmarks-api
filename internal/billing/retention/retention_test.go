package retention

import (
	"testing"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/billing/domain"
)

// mockDB implements the retention database interface for testing
type mockDB struct {
	attempts []DeliveryAttempt
}

func (m *mockDB) GetOldDeliveryAttempts(plan domain.Plan, olderThan time.Time) ([]DeliveryAttempt, error) {
	var result []DeliveryAttempt
	for _, a := range m.attempts {
		if a.Plan == plan && a.CreatedAt.Before(olderThan) {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *mockDB) DeleteDeliveryAttempts(ids []int64) (int64, error) {
	deleted := int64(0)
	for _, id := range ids {
		for i, a := range m.attempts {
			if a.ID == id {
				m.attempts = append(m.attempts[:i], m.attempts[i+1:]...)
				deleted++
				break
			}
		}
	}
	return deleted, nil
}

func TestRetentionService_DeletesOldAttempts(t *testing.T) {
	now := time.Now()
	freePlan := domain.PlanFree
	proPlan := domain.PlanPro

	// Free: 7 days, Pro: 90 days
	freeCutoff := now.AddDate(0, 0, -8)  // 8 days ago (older than 7)
	proCutoff := now.AddDate(0, 0, -91)  // 91 days ago (older than 90)

	tests := []struct {
		name           string
		plan           domain.Plan
		cutoff         time.Time
		setupAttempts  []DeliveryAttempt
		expectedDeleted int64
		expectedRemaining int
	}{
		{
			name: "Free plan - deletes attempts older than 7 days",
			plan: freePlan,
			cutoff: freeCutoff,
			setupAttempts: []DeliveryAttempt{
				{ID: 1, UserID: "user-1", Plan: freePlan, CreatedAt: now.AddDate(0, 0, -10)}, // 10 days - should delete
				{ID: 2, UserID: "user-1", Plan: freePlan, CreatedAt: now.AddDate(0, 0, -5)},  // 5 days - keep
				{ID: 3, UserID: "user-2", Plan: freePlan, CreatedAt: now.AddDate(0, 0, -15)}, // 15 days - should delete
			},
			expectedDeleted: 2,
			expectedRemaining: 1,
		},
		{
			name: "Pro plan - deletes attempts older than 90 days",
			plan: proPlan,
			cutoff: proCutoff,
			setupAttempts: []DeliveryAttempt{
				{ID: 10, UserID: "user-3", Plan: proPlan, CreatedAt: now.AddDate(0, 0, -100)}, // 100 days - should delete
				{ID: 11, UserID: "user-3", Plan: proPlan, CreatedAt: now.AddDate(0, 0, -60)},  // 60 days - keep
				{ID: 12, UserID: "user-4", Plan: proPlan, CreatedAt: now.AddDate(0, 0, -120)}, // 120 days - should delete
			},
			expectedDeleted: 2,
			expectedRemaining: 1,
		},
		{
			name: "Free plan - no old attempts to delete",
			plan: freePlan,
			cutoff: freeCutoff,
			setupAttempts: []DeliveryAttempt{
				{ID: 20, UserID: "user-5", Plan: freePlan, CreatedAt: now.AddDate(0, 0, -2)},
				{ID: 21, UserID: "user-5", Plan: freePlan, CreatedAt: now.AddDate(0, 0, -1)},
			},
			expectedDeleted: 0,
			expectedRemaining: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDB{attempts: tt.setupAttempts}
			svc := NewRetentionService(mock)

			deleted, err := svc.Cleanup(tt.plan, tt.cutoff)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if deleted != tt.expectedDeleted {
				t.Errorf("expected %d deleted, got %d", tt.expectedDeleted, deleted)
			}
			if len(mock.attempts) != tt.expectedRemaining {
				t.Errorf("expected %d remaining, got %d", tt.expectedRemaining, len(mock.attempts))
			}
		})
	}
}

func TestRetentionService_CutoffCalculation(t *testing.T) {
	svc := NewRetentionService(nil)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	freeCutoff := svc.CutoffFor(domain.PlanFree, now)
	proCutoff := svc.CutoffFor(domain.PlanPro, now)

	expectedFree := now.AddDate(0, 0, -7)
	expectedPro := now.AddDate(0, 0, -90)

	if !freeCutoff.Equal(expectedFree) {
		t.Errorf("Free cutoff: expected %v, got %v", expectedFree, freeCutoff)
	}
	if !proCutoff.Equal(expectedPro) {
		t.Errorf("Pro cutoff: expected %v, got %v", expectedPro, proCutoff)
	}
}

func TestRetentionService_RunsForAllPlans(t *testing.T) {
	now := time.Now()
	mock := &mockDB{
		attempts: []DeliveryAttempt{
			{ID: 1, UserID: "user-1", Plan: domain.PlanFree, CreatedAt: now.AddDate(0, 0, -10)},
			{ID: 2, UserID: "user-2", Plan: domain.PlanPro, CreatedAt: now.AddDate(0, 0, -100)},
			{ID: 3, UserID: "user-3", Plan: domain.PlanFree, CreatedAt: now.AddDate(0, 0, -2)},
			{ID: 4, UserID: "user-4", Plan: domain.PlanPro, CreatedAt: now.AddDate(0, 0, -50)},
		},
	}
	svc := NewRetentionService(mock)

	result, err := svc.CleanupAll(now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.DeletedFree != 1 {
		t.Errorf("expected 1 Free deleted, got %d", result.DeletedFree)
	}
	if result.DeletedPro != 1 {
		t.Errorf("expected 1 Pro deleted, got %d", result.DeletedPro)
	}
	if result.TotalDeleted != 2 {
		t.Errorf("expected 2 total deleted, got %d", result.TotalDeleted)
	}
	if len(mock.attempts) != 2 {
		t.Errorf("expected 2 remaining, got %d", len(mock.attempts))
	}
}