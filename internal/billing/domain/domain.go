package domain

import (
	"errors"
	"strings"
	"time"
)

// Plan represents a billing plan tier.
type Plan int

const (
	PlanFree Plan = iota
	PlanPro
)

// String returns the lowercase string representation of the plan.
func (p Plan) String() string {
	switch p {
	case PlanFree:
		return "free"
	case PlanPro:
		return "pro"
	default:
		return "free"
	}
}

// ParsePlan parses a string into a Plan. Returns error for unknown plans.
func ParsePlan(s string) (Plan, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "free":
		return PlanFree, nil
	case "pro":
		return PlanPro, nil
	default:
		return PlanFree, errors.New("unknown plan: " + s)
	}
}

// ParsePlanOrDefault parses a string into a Plan, defaulting to Free on error.
func ParsePlanOrDefault(s string) Plan {
	plan, _ := ParsePlan(s)
	return plan
}

// QuotaLimits defines the resource limits for a plan.
type QuotaLimits struct {
	MaxWebhooks         int
	MaxRequestsPerPeriod int
	RetentionDays       int
	PriceBRLCents       int
	PriceUSDCents       int
}

// QuotasFor returns the quota limits for a given plan.
// Unknown plans return Free quotas as a safe default.
func QuotasFor(p Plan) QuotaLimits {
	switch p {
	case PlanPro:
		return QuotaLimits{
			MaxWebhooks:         50,
			MaxRequestsPerPeriod: 1_000_000,
			RetentionDays:       90,
			PriceBRLCents:       4900, // R$ 49,00
			PriceUSDCents:       1900, // $19,00
		}
	default: // PlanFree and unknown
		return QuotaLimits{
			MaxWebhooks:         3,
			MaxRequestsPerPeriod: 10_000,
			RetentionDays:       7,
			PriceBRLCents:       0,
			PriceUSDCents:       0,
		}
	}
}

// SubscriptionStatus represents the state of a user's subscription.
type SubscriptionStatus int

const (
	SubscriptionStatusNone       SubscriptionStatus = iota // No subscription (Free tier)
	SubscriptionStatusIncomplete                           // Checkout started, not completed
	SubscriptionStatusActive                               // Active paid subscription
	SubscriptionStatusPastDue                              // Payment failed, in retry
	SubscriptionStatusCanceled                             // Canceled (may have access until period_end)
)

// String returns the string representation of the status.
func (s SubscriptionStatus) String() string {
	switch s {
	case SubscriptionStatusNone:
		return "none"
	case SubscriptionStatusIncomplete:
		return "incomplete"
	case SubscriptionStatusActive:
		return "active"
	case SubscriptionStatusPastDue:
		return "past_due"
	case SubscriptionStatusCanceled:
		return "canceled"
	default:
		return "none"
	}
}

// Subscription represents a user's subscription state.
type Subscription struct {
	Status            SubscriptionStatus
	CurrentPeriodEnd  time.Time
	CancelAtPeriodEnd bool
}

// NewSubscription creates a new Subscription with the given status and period end.
func NewSubscription(status SubscriptionStatus, periodEnd time.Time) *Subscription {
	return &Subscription{
		Status:           status,
		CurrentPeriodEnd: periodEnd,
	}
}

// Transition updates the subscription status based on a Stripe event type.
// Returns error if the transition is invalid.
func (s *Subscription) Transition(eventType string) error {
	switch eventType {
	case "checkout.session.completed":
		if s.Status == SubscriptionStatusIncomplete || s.Status == SubscriptionStatusNone {
			s.Status = SubscriptionStatusActive
			return nil
		}
		return errors.New("invalid transition to active from " + s.Status.String())

	case "invoice.payment_succeeded":
		if s.Status == SubscriptionStatusActive || s.Status == SubscriptionStatusPastDue {
			s.Status = SubscriptionStatusActive
			return nil
		}
		return errors.New("invalid transition to active from " + s.Status.String())

	case "invoice.payment_failed":
		if s.Status == SubscriptionStatusActive || s.Status == SubscriptionStatusPastDue {
			s.Status = SubscriptionStatusPastDue
			return nil
		}
		return errors.New("invalid transition to past_due from " + s.Status.String())

	case "customer.subscription.deleted":
		if s.Status == SubscriptionStatusActive || s.Status == SubscriptionStatusPastDue {
			s.Status = SubscriptionStatusCanceled
			return nil
		}
		return errors.New("invalid transition to canceled from " + s.Status.String())

	default:
		return errors.New("unknown event type: " + eventType)
	}
}

// IsActive returns true if the subscription currently grants Pro access.
// Uses the given reference time (typically time.Now()).
func (s *Subscription) IsActive(refTime time.Time) bool {
	switch s.Status {
	case SubscriptionStatusActive, SubscriptionStatusPastDue:
		return refTime.Before(s.CurrentPeriodEnd)
	case SubscriptionStatusCanceled:
		if s.CancelAtPeriodEnd {
			return refTime.Before(s.CurrentPeriodEnd)
		}
		return false
	default:
		return false
	}
}

// CanCreateWebhook returns true if the user can create another webhook under their plan's quota.
func CanCreateWebhook(q QuotaLimits, currentCount int) bool {
	return currentCount < q.MaxWebhooks
}

// CanDeliverRequest returns true if the user can make another request under their plan's quota.
func CanDeliverRequest(q QuotaLimits, currentCount int) bool {
	return currentCount < q.MaxRequestsPerPeriod
}