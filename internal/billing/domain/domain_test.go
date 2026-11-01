package domain

import (
	"testing"
	"time"
)

func TestPlanQuotas(t *testing.T) {
	tests := []struct {
		name           string
		plan           Plan
		maxWebhooks    int
		maxRequests    int
		retentionDays  int
		priceBRLCents  int
		priceUSDCents  int
	}{
		{
			name:           "Free plan has correct quotas",
			plan:           PlanFree,
			maxWebhooks:    3,
			maxRequests:    10_000,
			retentionDays:  7,
			priceBRLCents:  0,
			priceUSDCents:  0,
		},
		{
			name:           "Pro plan has correct quotas",
			plan:           PlanPro,
			maxWebhooks:    50,
			maxRequests:    1_000_000,
			retentionDays:  90,
			priceBRLCents:  4900, // R$ 49,00
			priceUSDCents:  1900, // $19,00
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := QuotasFor(tt.plan)

			if q.MaxWebhooks != tt.maxWebhooks {
				t.Errorf("MaxWebhooks: expected %d, got %d", tt.maxWebhooks, q.MaxWebhooks)
			}
			if q.MaxRequestsPerPeriod != tt.maxRequests {
				t.Errorf("MaxRequestsPerPeriod: expected %d, got %d", tt.maxRequests, q.MaxRequestsPerPeriod)
			}
			if q.RetentionDays != tt.retentionDays {
				t.Errorf("RetentionDays: expected %d, got %d", tt.retentionDays, q.RetentionDays)
			}
			if q.PriceBRLCents != tt.priceBRLCents {
				t.Errorf("PriceBRLCents: expected %d, got %d", tt.priceBRLCents, q.PriceBRLCents)
			}
			if q.PriceUSDCents != tt.priceUSDCents {
				t.Errorf("PriceUSDCents: expected %d, got %d", tt.priceUSDCents, q.PriceUSDCents)
			}
		})
	}
}

func TestPlanString(t *testing.T) {
	if PlanFree.String() != "free" {
		t.Errorf("PlanFree.String() = %q, want %q", PlanFree.String(), "free")
	}
	if PlanPro.String() != "pro" {
		t.Errorf("PlanPro.String() = %q, want %q", PlanPro.String(), "pro")
	}
}

func TestParsePlan(t *testing.T) {
	tests := []struct {
		input    string
		expected Plan
		hasError bool
	}{
		{"free", PlanFree, false},
		{"pro", PlanPro, false},
		{"FREE", PlanFree, false},
		{"PRO", PlanPro, false},
		{"unknown", PlanFree, true},
		{"", PlanFree, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			p, err := ParsePlan(tt.input)
			if tt.hasError {
				if err == nil {
					t.Error("expected error for invalid plan, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if p != tt.expected {
					t.Errorf("ParsePlan(%q) = %v, want %v", tt.input, p, tt.expected)
				}
			}
		})
	}
}

func TestSubscriptionStatusTransitions(t *testing.T) {
	tests := []struct {
		name       string
		from       SubscriptionStatus
		event      string
		wantStatus SubscriptionStatus
		wantError  bool
	}{
		// checkout.session.completed -> Active
		{"incomplete to active on checkout completed", SubscriptionStatusIncomplete, "checkout.session.completed", SubscriptionStatusActive, false},
		// invoice.payment_succeeded keeps Active
		{"active stays active on payment succeeded", SubscriptionStatusActive, "invoice.payment_succeeded", SubscriptionStatusActive, false},
		{"past_due to active on payment succeeded", SubscriptionStatusPastDue, "invoice.payment_succeeded", SubscriptionStatusActive, false},
		// invoice.payment_failed -> PastDue
		{"active to past_due on payment failed", SubscriptionStatusActive, "invoice.payment_failed", SubscriptionStatusPastDue, false},
		{"past_due stays past_due on payment failed", SubscriptionStatusPastDue, "invoice.payment_failed", SubscriptionStatusPastDue, false},
		// customer.subscription.deleted -> Canceled
		{"active to canceled on subscription deleted", SubscriptionStatusActive, "customer.subscription.deleted", SubscriptionStatusCanceled, false},
		{"past_due to canceled on subscription deleted", SubscriptionStatusPastDue, "customer.subscription.deleted", SubscriptionStatusCanceled, false},
		// Invalid transitions
		{"canceled cannot transition", SubscriptionStatusCanceled, "invoice.payment_succeeded", SubscriptionStatusCanceled, true},
		{"none cannot transition directly", SubscriptionStatusNone, "invoice.payment_succeeded", SubscriptionStatusNone, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSubscription(tt.from, time.Now().Add(30*24*time.Hour))
			err := s.Transition(tt.event)

			if tt.wantError {
				if err == nil {
					t.Error("expected error for invalid transition, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if s.Status != tt.wantStatus {
					t.Errorf("Status after %s: expected %v, got %v", tt.event, tt.wantStatus, s.Status)
				}
			}
		})
	}
}

func TestSubscriptionIsActive(t *testing.T) {
	now := time.Now()
	future := now.Add(30 * 24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	tests := []struct {
		name       string
		status     SubscriptionStatus
		periodEnd  time.Time
		cancelAt   bool
		expected   bool
	}{
		{"active with future period end is active", SubscriptionStatusActive, future, false, true},
		{"active with past period end is inactive", SubscriptionStatusActive, past, false, false},
		{"past_due with future period end is active", SubscriptionStatusPastDue, future, false, true},
		{"past_due with past period end is inactive", SubscriptionStatusPastDue, past, false, false},
		{"canceled with cancel_at_period_end and future end is active", SubscriptionStatusCanceled, future, true, true},
		{"canceled with cancel_at_period_end and past end is inactive", SubscriptionStatusCanceled, past, true, false},
		{"canceled immediately is inactive", SubscriptionStatusCanceled, future, false, false},
		{"incomplete is inactive", SubscriptionStatusIncomplete, future, false, false},
		{"none is inactive", SubscriptionStatusNone, future, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSubscription(tt.status, tt.periodEnd)
			s.CancelAtPeriodEnd = tt.cancelAt
			if s.IsActive(now) != tt.expected {
				t.Errorf("IsActive() = %v, want %v", s.IsActive(now), tt.expected)
			}
		})
	}
}

func TestCanCreateWebhook(t *testing.T) {
	tests := []struct {
		name           string
		plan           Plan
		currentCount   int
		expectedAllowed bool
	}{
		{"free plan under limit", PlanFree, 2, true},
		{"free plan at limit", PlanFree, 3, false},
		{"free plan over limit", PlanFree, 4, false},
		{"pro plan under limit", PlanPro, 49, true},
		{"pro plan at limit", PlanPro, 50, false},
		{"pro plan over limit", PlanPro, 51, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := QuotasFor(tt.plan)
			allowed := CanCreateWebhook(q, tt.currentCount)
			if allowed != tt.expectedAllowed {
				t.Errorf("CanCreateWebhook(plan=%s, count=%d) = %v, want %v", tt.plan, tt.currentCount, allowed, tt.expectedAllowed)
			}
		})
	}
}

func TestCanDeliverRequest(t *testing.T) {
	tests := []struct {
		name           string
		plan           Plan
		currentCount   int
		expectedAllowed bool
	}{
		{"free plan under limit", PlanFree, 9_999, true},
		{"free plan at limit", PlanFree, 10_000, false},
		{"free plan over limit", PlanFree, 10_001, false},
		{"pro plan under limit", PlanPro, 999_999, true},
		{"pro plan at limit", PlanPro, 1_000_000, false},
		{"pro plan over limit", PlanPro, 1_000_001, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := QuotasFor(tt.plan)
			allowed := CanDeliverRequest(q, tt.currentCount)
			if allowed != tt.expectedAllowed {
				t.Errorf("CanDeliverRequest(plan=%s, count=%d) = %v, want %v", tt.plan, tt.currentCount, allowed, tt.expectedAllowed)
			}
		})
	}
}

func TestQuotasForUnknownPlan(t *testing.T) {
	// Unknown plan should return Free quotas (safe default)
	q := QuotasFor(Plan(999))
	freeQ := QuotasFor(PlanFree)
	if q != freeQ {
		t.Errorf("unknown plan should return Free quotas, got %+v", q)
	}
}