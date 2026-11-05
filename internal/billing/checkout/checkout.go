package checkout

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/Carlos20052030/bookmarks-api/internal/middleware"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v79"
	"github.com/stripe/stripe-go/v79/checkout/session"
)

type Handler struct {
	stripeKey  string
	baseURL    string
	logger     *slog.Logger
}

func NewHandler() *Handler {
	stripeKey := os.Getenv("STRIPE_SECRET_KEY")
	if stripeKey == "" {
		slog.Warn("STRIPE_SECRET_KEY not set, checkout will fail")
	}
	baseURL := os.Getenv("APP_BASE_URL")
	if baseURL == "" {
		baseURL = "https://app.nuntix.com.br"
	}
	return &Handler{
		stripeKey: stripeKey,
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		logger:    slog.Default().With("component", "checkout"),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	// Get user ID from context (set by Auth middleware)
	userID := middleware.UserIDFromContext(r.Context())
	if userID == uuid.Nil {
		http.Error(w, "Não autorizado", http.StatusUnauthorized)
		return
	}

	// Get price_id from query string
	priceID := r.URL.Query().Get("price_id")

	// If no price_id provided, select based on locale (from Accept-Language header)
	if priceID == "" {
		locale := r.Header.Get("Accept-Language")
		priceID = selectPriceIDByLocale(locale)
	}

	// Validate price_id format
	valid, validatedPriceID := parsePriceID(priceID)
	if !valid {
		http.Error(w, "Formato de price_id inválido", http.StatusBadRequest)
		return
	}
	priceID = validatedPriceID

	// Set Stripe key for this request
	stripe.Key = h.stripeKey

	// Create checkout session with user ID as ClientReferenceID
	sess, err := session.New(&stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL:        stripe.String("https://nuntix.com.br/billing?checkout=success"),
		CancelURL:         stripe.String("https://nuntix.com.br/billing?checkout=canceled"),
		ClientReferenceID: stripe.String(userID.String()),
	})

	if err != nil {
		h.logger.Error("failed to create checkout session", "error", err, "price_id", priceID)
		http.Error(w, "Falha ao criar sessão de checkout", http.StatusInternalServerError)
		return
	}

	// Return session URL
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url": sess.URL,
	})
}

// selectPriceIDByLocale selects the appropriate price ID based on user locale
func selectPriceIDByLocale(locale string) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	// Brazilian Portuguese -> BRL price
	if strings.HasPrefix(locale, "pt-br") || strings.HasPrefix(locale, "pt_br") || locale == "pt" {
		return os.Getenv("STRIPE_PRICE_ID_BRL")
	}
	// Default to USD price
	return os.Getenv("STRIPE_PRICE_ID_USD")
}

// parsePriceID validates and returns the price ID
// Stripe price IDs start with "price_"
func parsePriceID(priceID string) (bool, string) {
	priceID = strings.TrimSpace(priceID)
	if priceID == "" {
		return false, ""
	}
	// Basic validation: Stripe price IDs start with "price_"
	if !strings.HasPrefix(priceID, "price_") {
		return false, ""
	}
	return true, priceID
}