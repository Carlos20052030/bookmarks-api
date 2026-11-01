package usage

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/billing/domain"
	"github.com/Carlos20052030/bookmarks-api/internal/billing/quota"
)

// QuotaServiceInterface defines the interface for quota checking
type QuotaServiceInterface interface {
	GetCurrentUsage(userID string) quota.UserUsage
}

// DBInterface defines the interface for database operations
type DBInterface interface {
	GetUserBillingInfo(userID string) (domain.Plan, domain.SubscriptionStatus, sql.NullTime, int, error)
}

// UsageResponse represents the JSON response for the usage endpoint
type UsageResponse struct {
	Plan      string     `json:"plan"`
	PeriodEnd string     `json:"period_end"`
	Requests  UsageLimit `json:"requests"`
	Webhooks  UsageLimit `json:"webhooks"`
}

// UsageLimit represents a quota limit with used and limit values
type UsageLimit struct {
	Used  int `json:"used"`
	Limit int `json:"limit"`
}

type Handler struct {
	quotaSvc QuotaServiceInterface
	db       DBInterface
	logger   *slog.Logger
}

func NewHandler(quotaSvc QuotaServiceInterface, db DBInterface) *Handler {
	return &Handler{
		quotaSvc: quotaSvc,
		db:       db,
		logger:   slog.Default().With("component", "usage-api"),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get user ID from header (set by auth middleware)
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get user billing info from database
	plan, _, periodEnd, webhookCount, err := h.db.GetUserBillingInfo(userID)
	if err != nil {
		h.logger.Error("failed to get user billing info", "error", err, "user_id", userID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get current usage from quota service
	usage := h.quotaSvc.GetCurrentUsage(userID)

	// Get plan quotas
	quotas := domain.QuotasFor(plan)

	// Build response
	resp := UsageResponse{
		Plan: plan.String(),
		PeriodEnd: formatPeriodEnd(periodEnd),
		Requests: UsageLimit{
			Used:  usage.RequestCount,
			Limit: quotas.MaxRequestsPerPeriod,
		},
		Webhooks: UsageLimit{
			Used:  webhookCount,
			Limit: quotas.MaxWebhooks,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func formatPeriodEnd(periodEnd sql.NullTime) string {
	if periodEnd.Valid {
		return periodEnd.Time.Format(time.RFC3339)
	}
	return ""
}