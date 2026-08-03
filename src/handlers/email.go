package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ogen-app/ogen/src/email"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// SecretResolver resolves a named secret (e.g. email_link_secret) per call so a
// rotation takes effect with no restart. Satisfied by an adapter over
// secrets.Store in server wiring.
type SecretResolver func(ctx context.Context) (string, error)

// EmailHandler serves the public unsubscribe surface (CON-154 §10). Both the
// link (GET) and the RFC 8058 one-click (POST) verify a signed token — the
// token is the capability, so no auth is required — and upsert a marketing
// suppression for the address it authorises.
type EmailHandler struct {
	suppressions repository.EmailSuppressionRepository
	linkSecret   SecretResolver
}

// NewEmailHandler constructs the unsubscribe handler.
func NewEmailHandler(suppressions repository.EmailSuppressionRepository, linkSecret SecretResolver) *EmailHandler {
	return &EmailHandler{suppressions: suppressions, linkSecret: linkSecret}
}

// Register mounts the public unsubscribe routes (no auth — token-gated).
func (h *EmailHandler) Register(app *fiber.App) {
	app.Get("/api/email/unsubscribe", h.Unsubscribe)
	app.Post("/api/email/unsubscribe", h.UnsubscribeOneClick)
}

// Unsubscribe godoc
// @Summary      Unsubscribe from marketing email (link)
// @Description  Verifies a signed token and suppresses the address from
// @Description  marketing mail, then renders a small confirmation page.
// @Tags         email
// @Produce      html
// @Param        token  query  string  true  "Signed unsubscribe token"
// @Success      200  {string}  string  "Confirmation page"
// @Failure      400  {string}  string  "Invalid or expired token"
// @Router       /api/email/unsubscribe [get]
func (h *EmailHandler) Unsubscribe(c *fiber.Ctx) error {
	addr, err := h.resolve(c)
	if errors.Is(err, email.ErrInvalidToken) || errors.Is(err, email.ErrExpiredToken) {
		return c.Status(fiber.StatusBadRequest).Type("html").SendString(unsubscribeInvalidHTML)
	}
	if err != nil {
		return err // secret-read / server error → 500
	}
	if err := h.suppress(c.Context(), addr); err != nil {
		return err
	}
	return c.Status(fiber.StatusOK).Type("html").SendString(unsubscribeOKHTML)
}

// UnsubscribeOneClick godoc
// @Summary      One-click unsubscribe (RFC 8058)
// @Description  The POST target advertised in the List-Unsubscribe-Post header.
// @Tags         email
// @Param        token  query  string  false  "Signed unsubscribe token"
// @Success      200  "Unsubscribed"
// @Failure      400  {object}  map[string]string
// @Router       /api/email/unsubscribe [post]
func (h *EmailHandler) UnsubscribeOneClick(c *fiber.Ctx) error {
	addr, err := h.resolve(c)
	if errors.Is(err, email.ErrInvalidToken) || errors.Is(err, email.ErrExpiredToken) {
		return fiber.NewError(fiber.StatusBadRequest, "invalid or expired token")
	}
	if err != nil {
		return err
	}
	if err := h.suppress(c.Context(), addr); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusOK)
}

// resolve extracts the token (query or form) and returns the address it
// authorises. A missing/forged/expired token yields email.ErrInvalidToken /
// ErrExpiredToken; a secret-read failure returns that error (mapped to 500).
func (h *EmailHandler) resolve(c *fiber.Ctx) (string, error) {
	token := c.Query("token")
	if token == "" {
		token = c.FormValue("token")
	}
	if token == "" {
		return "", email.ErrInvalidToken
	}
	secret, err := h.linkSecret(c.Context())
	if err != nil {
		return "", err
	}
	if secret == "" {
		return "", email.ErrInvalidToken
	}
	return email.VerifyUnsubscribe(secret, token, time.Now().UTC())
}

func (h *EmailHandler) suppress(ctx context.Context, addr string) error {
	id, err := models.NewID()
	if err != nil {
		return err
	}
	return h.suppressions.Upsert(ctx, &models.EmailSuppression{
		ID:     id,
		Email:  repository.NormalizeEmail(addr),
		Scope:  models.EmailSuppressionScopeMarketing,
		Reason: models.EmailSuppressionReasonUnsubscribe,
		Source: models.EmailSuppressionSourceUser,
	})
}

const unsubscribeOKHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Unsubscribed</title></head>` +
	`<body style="font-family:Arial,sans-serif;max-width:480px;margin:80px auto;text-align:center;color:#1a1a1a;">` +
	`<h1 style="font-size:20px;">You're unsubscribed</h1>` +
	`<p style="color:#555;">You'll no longer receive marketing emails from Ogen. Essential account emails may still be sent.</p>` +
	`</body></html>`

const unsubscribeInvalidHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Invalid link</title></head>` +
	`<body style="font-family:Arial,sans-serif;max-width:480px;margin:80px auto;text-align:center;color:#1a1a1a;">` +
	`<h1 style="font-size:20px;">This link is no longer valid</h1>` +
	`<p style="color:#555;">The unsubscribe link is invalid or has expired. Please use the link in a more recent email.</p>` +
	`</body></html>`
