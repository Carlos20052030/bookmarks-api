package webhook

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Carlos20052030/bookmarks-api/internal/billing/domain"
	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/subscription"
	"github.com/stripe/stripe-go/v79/webhook"
)

type Handler struct {
	db        *sql.DB
	secret    string
	logger    *slog.Logger
}

func NewHandler(db *sql.DB) *Handler {
	secret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if secret == "" {
		slog.Warn("STRIPE_WEBHOOK_SECRET not set, webhook verification will fail")
	}
	return &Handler{
		db:     db,
		secret: secret,
		logger: slog.Default().With("component", "stripe-webhook"),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read raw body for Stripe signature verification
	// Must use io.ReadAll to get the exact bytes Stripe signed
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read body", "error", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Verify Stripe signature
	sigHeader := r.Header.Get("Stripe-Signature")
	if sigHeader == "" {
		h.logger.Warn("missing Stripe-Signature header")
		http.Error(w, "Missing Stripe-Signature header", http.StatusBadRequest)
		return
	}

	event, err := webhook.ConstructEventWithOptions(body, sigHeader, h.secret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		h.logger.Warn("invalid Stripe signature", "error", err)
		http.Error(w, "Invalid signature", http.StatusBadRequest)
		return
	}

	// Idempotency check - see if event already processed
	if h.db != nil {
		processed, err := h.isEventProcessed(event.ID)
		if err != nil {
			h.logger.Error("failed to check event idempotency", "error", err, "event_id", event.ID)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if processed {
			h.logger.Info("duplicate event, skipping", "event_id", event.ID, "type", event.Type)
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	// Process event based on type
	var processErr error
	switch event.Type {
	case "checkout.session.completed":
		processErr = h.handleCheckoutSessionCompleted(event)
	case "invoice.payment_succeeded":
		processErr = h.handleInvoicePaymentSucceeded(event)
	case "invoice.payment_failed":
		processErr = h.handleInvoicePaymentFailed(event)
	case "customer.subscription.deleted":
		processErr = h.handleCustomerSubscriptionDeleted(event)
	default:
		h.logger.Info("unhandled event type", "type", event.Type)
	}

	// Record event in audit log
	if h.db != nil {
		if err := h.recordEvent(event, processErr == nil); err != nil {
			h.logger.Error("failed to record event", "error", err, "event_id", event.ID)
			// Don't return error to Stripe - we processed it
		}
	}

	if processErr != nil {
		h.logger.Error("failed to process event", "error", processErr, "event_id", event.ID, "type", event.Type)
		http.Error(w, "Event processing failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) isEventProcessed(eventID string) (bool, error) {
	var exists bool
	err := h.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM stripe_events WHERE event_id = $1)",
		eventID,
	).Scan(&exists)
	return exists, err
}

func (h *Handler) recordEvent(event stripe.Event, success bool) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = h.db.Exec(
		`INSERT INTO stripe_events (event_id, event_type, payload_json, processed_at, success)
		 VALUES ($1, $2, $3, NOW(), $4)
		 ON CONFLICT (event_id) DO NOTHING`,
		event.ID, event.Type, payload, success,
	)
	return err
}

func (h *Handler) handleCheckoutSessionCompleted(event stripe.Event) error {
	var session stripe.CheckoutSession
	if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
		return err
	}

	if session.Customer == nil || session.Subscription == nil {
		return nil // Not a subscription checkout
	}

	// Get user ID from ClientReferenceID (set during checkout creation)
	userID := session.ClientReferenceID
	if userID == "" {
		h.logger.Warn("checkout.session.completed: missing ClientReferenceID")
		return nil // Can't identify user, skip
	}

	customerID := session.Customer.ID
	subscriptionID := session.Subscription.ID

	// Fetch the subscription to get current_period_end
	sub, err := subscription.Get(subscriptionID, nil)
	if err != nil {
		h.logger.Error("checkout.session.completed: failed to fetch subscription", "error", err, "subscription", subscriptionID)
		return err
	}

	periodEnd := time.Unix(sub.CurrentPeriodEnd, 0)

	// Update user by ID (from ClientReferenceID)
	result, err := h.db.Exec(
		`UPDATE users
		 SET plan = $1, subscription_status = $2, stripe_customer_id = $3, current_period_end = $4
		 WHERE id = $5`,
		domain.PlanPro.String(), domain.SubscriptionStatusActive.String(), customerID, periodEnd, userID,
	)

	if err != nil {
		h.logger.Error("checkout.session.completed: failed to update user", "error", err, "user_id", userID, "customer", customerID)
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		h.logger.Warn("checkout.session.completed: no user found with ID", "user_id", userID)
		return nil
	}

	h.logger.Info("checkout completed", "user_id", userID, "customer", customerID, "subscription", subscriptionID, "period_end", periodEnd)
	return nil
}

func (h *Handler) handleInvoicePaymentSucceeded(event stripe.Event) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
		return err
	}

	if invoice.Customer == nil || invoice.Subscription == nil {
		return nil
	}

	customerID := invoice.Customer.ID
	periodEnd := time.Unix(invoice.PeriodEnd, 0)

	_, err := h.db.Exec(
		`UPDATE users
		 SET subscription_status = $1, current_period_end = $2
		 WHERE stripe_customer_id = $3`,
		domain.SubscriptionStatusActive.String(), periodEnd, customerID,
	)

	h.logger.Info("invoice payment succeeded", "customer", customerID, "period_end", periodEnd)
	return err
}

func (h *Handler) handleInvoicePaymentFailed(event stripe.Event) error {
	var invoice stripe.Invoice
	if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
		return err
	}

	if invoice.Customer == nil {
		return nil
	}

	customerID := invoice.Customer.ID

	_, err := h.db.Exec(
		`UPDATE users
		 SET subscription_status = $1
		 WHERE stripe_customer_id = $2`,
		domain.SubscriptionStatusPastDue.String(), customerID,
	)

	h.logger.Info("invoice payment failed", "customer", customerID, "attempt", invoice.AttemptCount)
	return err
}

func (h *Handler) handleCustomerSubscriptionDeleted(event stripe.Event) error {
	var subscription stripe.Subscription
	if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
		return err
	}

	if subscription.Customer == nil {
		return nil
	}

	customerID := subscription.Customer.ID

	// Check if cancel_at_period_end
	if subscription.CancelAtPeriodEnd {
		// Keep active until period end - status stays active, but mark canceled
		_, err := h.db.Exec(
			`UPDATE users
			 SET subscription_status = $1
			 WHERE stripe_customer_id = $2`,
			domain.SubscriptionStatusCanceled.String(), customerID,
		)
		return err
	}

	// Immediate cancellation - downgrade to free
	_, err := h.db.Exec(
		`UPDATE users
		 SET plan = $1, subscription_status = $2, current_period_end = NULL
		 WHERE stripe_customer_id = $3`,
		domain.PlanFree.String(), domain.SubscriptionStatusCanceled.String(), customerID,
	)

	h.logger.Info("subscription deleted", "customer", customerID, "cancel_at_period_end", subscription.CancelAtPeriodEnd)
	return err
}