package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ResendClient is a client for sending emails via the Resend API.
type ResendClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewResendClient creates a new Resend client.
// If baseURL is empty, it defaults to the official Resend API URL.
func NewResendClient(apiKey, baseURL string) *ResendClient {
	if baseURL == "" {
		baseURL = "https://api.resend.com"
	}
	return &ResendClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendPasswordResetEmail sends a password reset email to the given address.
// The email contains a link to reset the password with the provided token.
func (c *ResendClient) SendPasswordResetEmail(ctx context.Context, email, token string) error {
	resetURL := fmt.Sprintf("https://nuntix.com.br/reset-password?token=%s", token)

	payload := map[string]any{
		"from":    "Nuntix <no-reply@nuntix.com.br>",
		"to":      []string{email},
		"subject": "Redefina sua senha no Nuntix",
		"html": fmt.Sprintf(`
			<!DOCTYPE html>
			<html>
			<head>
				<meta charset="utf-8">
				<meta name="viewport" content="width=device-width, initial-scale=1.0">
			</head>
			<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px;">
				<div style="background: #f8f9fa; border-radius: 8px; padding: 32px;">
					<h1 style="color: #1a1a2e; margin-top: 0; font-size: 24px;">Redefina sua senha</h1>
					<p style="font-size: 16px;">Olá,</p>
					<p style="font-size: 16px;">Recebemos uma solicitação para redefinir a senha da sua conta no Nuntix. Clique no botão abaixo para criar uma nova senha:</p>
					<div style="text-align: center; margin: 32px 0;">
						<a href="%s" style="background: #2563eb; color: white; padding: 14px 28px; text-decoration: none; border-radius: 6px; font-weight: 600; display: inline-block;">Redefinir senha</a>
					</div>
					<p style="font-size: 14px; color: #666;">Se o botão não funcionar, você pode copiar e colar este link no seu navegador:</p>
					<p style="font-size: 14px; color: #2563eb; word-break: break-all;">%s</p>
					<hr style="border: none; border-top: 1px solid #e0e0e0; margin: 24px 0;">
					<p style="font-size: 12px; color: #999;">Este link expira em 1 hora. Se você não solicitou esta redefinição, pode ignorar este e-mail com segurança.</p>
					<p style="font-size: 12px; color: #999;">— A equipe do Nuntix</p>
				</div>
			</body>
			</html>
		`, resetURL, resetURL),
		"text": fmt.Sprintf(`
Redefina sua senha no Nuntix

Olá,

Recebemos uma solicitação para redefinir a senha da sua conta no Nuntix. Acesse o link abaixo para criar uma nova senha:

%s

Este link expira em 1 hora. Se você não solicitou esta redefinição, pode ignorar este e-mail com segurança.

— A equipe do Nuntix
		`, resetURL),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal email payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/emails", bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr map[string]string
		if err := json.Unmarshal(body, &apiErr); err == nil {
			if msg, ok := apiErr["message"]; ok {
				return errors.New(msg)
			}
		}
		return fmt.Errorf("resend API error: %s", string(body))
	}

	return nil
}