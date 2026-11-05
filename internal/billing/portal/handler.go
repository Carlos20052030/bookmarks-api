package portal

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/Carlos20052030/bookmarks-api/internal/middleware"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/billingportal/session"
)

type Handler struct {
	db     *sql.DB
	logger *slog.Logger
}

func NewHandler(db *sql.DB) *Handler {
	return &Handler{
		db:     db,
		logger: slog.Default().With("component", "portal"),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var customerID sql.NullString
	err := h.db.QueryRowContext(r.Context(),
		"SELECT stripe_customer_id FROM users WHERE id = $1",
		userID,
	).Scan(&customerID)

	if err != nil {
		if err == sql.ErrNoRows {
			h.logger.Warn("user not found", "user_id", userID)
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		h.logger.Error("failed to query user", "error", err, "user_id", userID)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if !customerID.Valid {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "Nenhuma assinatura ativa encontrada. Faça o upgrade primeiro.",
		})
		return
	}

	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")

	sess, err := session.New(&stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID.String),
		ReturnURL: stripe.String("https://nuntix.com.br/billing"),
	})
	if err != nil {
		h.logger.Error("failed to create portal session", "error", err, "customer_id", customerID.String)
		http.Error(w, "Failed to create portal session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"url": sess.URL,
	})
}