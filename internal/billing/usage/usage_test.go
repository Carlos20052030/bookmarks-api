package usage

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/billing/domain"
	"github.com/Carlos20052030/bookmarks-api/internal/billing/quota"
)

type mockQuotaService struct {
	usage quota.UserUsage
}

func (m *mockQuotaService) CanCreateWebhook(userID string, plan domain.Plan, currentCount int) (bool, error) {
	return true, nil
}
func (m *mockQuotaService) CanDeliverRequest(userID string, plan domain.Plan, currentCount int) (bool, error) {
	return true, nil
}
func (m *mockQuotaService) ResetUser(userID string) {}
func (m *mockQuotaService) GetCurrentUsage(userID string) quota.UserUsage {
	return m.usage
}

type mockDB struct {
	userPlan           domain.Plan
	userStatus         domain.SubscriptionStatus
	userPeriodEnd      sql.NullTime
	activeWebhookCount int
}

func (m *mockDB) GetUserBillingInfo(ctx context.Context, userID string) (domain.Plan, domain.SubscriptionStatus, sql.NullTime, int, error) {
	return m.userPlan, m.userStatus, m.userPeriodEnd, m.activeWebhookCount, nil
}

func TestUsageHandler_ReturnsCorrectJSON(t *testing.T) {
	now := time.Now()
	periodEnd := now.Add(30 * 24 * time.Hour)

	tests := []struct {
		name           string
		plan           domain.Plan
		status         domain.SubscriptionStatus
		periodEnd      sql.NullTime
		webhookCount   int
		requestCount   int
		expectedPlan   string
		expectedReqLim int
		expectedReqUsed int
		expectedWhLim  int
		expectedWhUsed int
		expectedPeriodEnd string
	}{
		{
			name:           "Free plan user",
			plan:           domain.PlanFree,
			status:         domain.SubscriptionStatusNone,
			periodEnd:      sql.NullTime{Time: periodEnd, Valid: true},
			webhookCount:   2,
			requestCount:   5000,
			expectedPlan:   "free",
			expectedReqLim: 10000,
			expectedReqUsed: 5000,
			expectedWhLim:  3,
			expectedWhUsed: 2,
			expectedPeriodEnd: periodEnd.Format(time.RFC3339),
		},
		{
			name:           "Pro plan user",
			plan:           domain.PlanPro,
			status:         domain.SubscriptionStatusActive,
			periodEnd:      sql.NullTime{Time: periodEnd, Valid: true},
			webhookCount:   15,
			requestCount:   500000,
			expectedPlan:   "pro",
			expectedReqLim: 1000000,
			expectedReqUsed: 500000,
			expectedWhLim:  50,
			expectedWhUsed: 15,
			expectedPeriodEnd: periodEnd.Format(time.RFC3339),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockQS := &mockQuotaService{
				usage: quota.UserUsage{
					WebhookCount:  tt.webhookCount,
					RequestCount:  tt.requestCount,
					LastUpdatedAt: time.Now(),
				},
			}
			mockDB := &mockDB{
				userPlan:           tt.plan,
				userStatus:         tt.status,
				userPeriodEnd:      tt.periodEnd,
				activeWebhookCount: tt.webhookCount,
			}

			handler := NewHandler(mockQS, mockDB)

			req := httptest.NewRequest(http.MethodGet, "/api/billing/usage", nil)
			req.Header.Set("X-User-ID", "user-123")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", rr.Code)
				return
			}

			var resp UsageResponse
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if resp.Plan != tt.expectedPlan {
				t.Errorf("plan: expected %s, got %s", tt.expectedPlan, resp.Plan)
			}
			if resp.Requests.Limit != tt.expectedReqLim {
				t.Errorf("requests.limit: expected %d, got %d", tt.expectedReqLim, resp.Requests.Limit)
			}
			if resp.Requests.Used != tt.expectedReqUsed {
				t.Errorf("requests.used: expected %d, got %d", tt.expectedReqUsed, resp.Requests.Used)
			}
			if resp.Webhooks.Limit != tt.expectedWhLim {
				t.Errorf("webhooks.limit: expected %d, got %d", tt.expectedWhLim, resp.Webhooks.Limit)
			}
			if resp.Webhooks.Used != tt.expectedWhUsed {
				t.Errorf("webhooks.used: expected %d, got %d", tt.expectedWhUsed, resp.Webhooks.Used)
			}
			if resp.PeriodEnd != tt.expectedPeriodEnd {
				t.Errorf("period_end: expected %s, got %s", tt.expectedPeriodEnd, resp.PeriodEnd)
			}
		})
	}
}

func TestUsageHandler_RequiresAuth(t *testing.T) {
	handler := NewHandler(&mockQuotaService{}, &mockDB{})

	req := httptest.NewRequest(http.MethodGet, "/api/billing/usage", nil)
	// No X-User-ID header
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing user ID, got %d", rr.Code)
	}
}

func TestUsageHandler_OnlyGETAllowed(t *testing.T) {
	handler := NewHandler(&mockQuotaService{}, &mockDB{})

	req := httptest.NewRequest(http.MethodPost, "/api/billing/usage", nil)
	req.Header.Set("X-User-ID", "user-123")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", rr.Code)
	}
}

func TestUsageResponse_JSONStructure(t *testing.T) {
	resp := UsageResponse{
		Plan:      "pro",
		PeriodEnd: "2026-12-01T00:00:00Z",
		Requests:  UsageLimit{Used: 12345, Limit: 1000000},
		Webhooks:  UsageLimit{Used: 10, Limit: 50},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed["plan"] != "pro" {
		t.Error("plan missing in JSON")
	}
	if parsed["period_end"] == nil {
		t.Error("period_end missing in JSON")
	}
	if parsed["requests"] == nil {
		t.Error("requests missing in JSON")
	}
	if parsed["webhooks"] == nil {
		t.Error("webhooks missing in JSON")
	}

	req := parsed["requests"].(map[string]interface{})
	if req["used"] != float64(12345) || req["limit"] != float64(1000000) {
		t.Errorf("requests object incorrect: %v", req)
	}

	wh := parsed["webhooks"].(map[string]interface{})
	if wh["used"] != float64(10) || wh["limit"] != float64(50) {
		t.Errorf("webhooks object incorrect: %v", wh)
	}
}