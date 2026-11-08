package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Carlos20052030/bookmarks-api/internal/email"
)

// ContactHandler holds the dependencies for the contact endpoint.
type ContactHandler struct {
	emailClient *email.ResendClient
	logger      *slog.Logger
}

func NewContactHandler(emailClient *email.ResendClient, logger *slog.Logger) *ContactHandler {
	return &ContactHandler{emailClient: emailClient, logger: logger}
}

// contactRequest represents the incoming contact form data.
type contactRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Message string `json:"message"`
}

// Contact handles the contact form submission.
// It validates the input and sends an email to the support address.
func (h *ContactHandler) Contact(w http.ResponseWriter, r *http.Request) {
	var req contactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is not valid json")
		return
	}

	// Validate required fields
	name := strings.TrimSpace(req.Name)
	email := strings.TrimSpace(req.Email)
	message := strings.TrimSpace(req.Message)

	if name == "" || email == "" || message == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "name, email and message are required")
		return
	}

	// Send email if client is configured
	if h.emailClient != nil {
		go func() {
			ctx := context.Background()
			subject := "[Nuntix Contato] - " + name
			body := "De: " + email + "\n\n" + message
			if err := h.emailClient.SendContactEmail(ctx, email, subject, body); err != nil {
				h.logger.Error("send contact email", "err", err, "from_email", email)
			}
		}()
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Mensagem enviada com sucesso"})
}