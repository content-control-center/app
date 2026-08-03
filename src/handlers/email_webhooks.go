package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// resendWebhookTolerance bounds how far a webhook timestamp may drift from now
// before it's rejected as stale (replay protection). Svix's own default.
const resendWebhookTolerance = 5 * time.Minute

var errInvalidSignature = errors.New("resend webhook: invalid signature")

// ResendWebhookHandler ingests Resend delivery events (CON-154 FR8). Resend
// signs webhooks with Svix, so every request is signature-verified before any
// side effect. Hard bounces and complaints auto-suppress the address (scope
// all); delivery events update the matching email_logs row.
type ResendWebhookHandler struct {
	suppressions  repository.EmailSuppressionRepository
	logs          repository.EmailLogRepository
	webhookSecret SecretResolver
}

// NewResendWebhookHandler constructs the webhook handler.
func NewResendWebhookHandler(suppressions repository.EmailSuppressionRepository, logs repository.EmailLogRepository, webhookSecret SecretResolver) *ResendWebhookHandler {
	return &ResendWebhookHandler{suppressions: suppressions, logs: logs, webhookSecret: webhookSecret}
}

// Register mounts the public (signature-gated) webhook route.
func (h *ResendWebhookHandler) Register(app *fiber.App) {
	app.Post("/api/webhooks/resend", h.Handle)
}

// resendEvent is the subset of the Resend/Svix event envelope we act on.
type resendEvent struct {
	Type string `json:"type"`
	Data struct {
		EmailID string   `json:"email_id"`
		To      []string `json:"to"`
	} `json:"data"`
}

// Handle godoc
// @Summary      Resend delivery webhook
// @Description  Signature-verified ingest of Resend events. Bounces/complaints
// @Description  suppress the address; delivery events update the send audit.
// @Tags         email
// @Accept       json
// @Param        svix-id         header  string  true  "Svix message id"
// @Param        svix-timestamp  header  string  true  "Svix timestamp"
// @Param        svix-signature  header  string  true  "Svix signature"
// @Success      200  "Acknowledged"
// @Failure      401  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/webhooks/resend [post]
func (h *ResendWebhookHandler) Handle(c *fiber.Ctx) error {
	secret, err := h.webhookSecret(c.Context())
	if err != nil {
		return err // secret-read failure → 500
	}
	if secret == "" {
		// Fail closed: without a secret we can't verify, so refuse rather than
		// trust an unauthenticated caller.
		return fiber.NewError(fiber.StatusServiceUnavailable, "resend webhook not configured")
	}

	body := c.Body()
	if err := verifySvixSignature(secret, c.Get("svix-id"), c.Get("svix-timestamp"), c.Get("svix-signature"), body, time.Now()); err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid signature")
	}

	var evt resendEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid payload")
	}

	ctx := c.Context()
	switch evt.Type {
	case "email.bounced":
		h.suppressAll(ctx, evt.Data.To, models.EmailSuppressionReasonBounce)
		h.markStatus(ctx, evt.Data.EmailID, models.EmailLogBounced)
	case "email.complained":
		h.suppressAll(ctx, evt.Data.To, models.EmailSuppressionReasonComplaint)
		h.markStatus(ctx, evt.Data.EmailID, models.EmailLogComplained)
	default:
		// delivered / opened / clicked / delivery_delayed etc.: acknowledged as
		// a no-op so Resend stops retrying. (Fine-grained delivery status is a
		// future enhancement.)
		slog.InfoContext(ctx, "resend webhook ignored", logging.AttrComponent, "handlers.resend_webhook", "type", evt.Type)
	}

	return c.SendStatus(fiber.StatusOK)
}

// suppressAll adds an all-scope suppression for each address (best-effort: a
// per-address failure is logged, not fatal to the ack).
func (h *ResendWebhookHandler) suppressAll(ctx context.Context, addrs []string, reason models.EmailSuppressionReason) {
	for _, a := range addrs {
		if a == "" {
			continue
		}
		id, err := models.NewID()
		if err != nil {
			slog.WarnContext(ctx, "suppression id gen failed", logging.AttrComponent, "handlers.resend_webhook", logging.AttrError, err)
			continue
		}
		if err := h.suppressions.Upsert(ctx, &models.EmailSuppression{
			ID:     id,
			Email:  repository.NormalizeEmail(a),
			Scope:  models.EmailSuppressionScopeAll,
			Reason: reason,
			Source: models.EmailSuppressionSourceWebhook,
		}); err != nil {
			slog.WarnContext(ctx, "suppression upsert failed", logging.AttrComponent, "handlers.resend_webhook", logging.AttrError, err)
		}
	}
}

func (h *ResendWebhookHandler) markStatus(ctx context.Context, providerMessageID string, status models.EmailLogStatus) {
	if providerMessageID == "" || h.logs == nil {
		return
	}
	if _, err := h.logs.UpdateStatusByProviderMessageID(ctx, providerMessageID, status); err != nil {
		slog.WarnContext(ctx, "email_log status update failed", logging.AttrComponent, "handlers.resend_webhook", logging.AttrError, err)
	}
}

// verifySvixSignature validates a Svix-signed webhook (the scheme Resend uses).
// signedContent = "<id>.<timestamp>.<body>"; the expected signature is the
// base64 HMAC-SHA256 of that content keyed by the (base64-decoded) secret. The
// svix-signature header is a space-separated list of "v1,<sig>" entries; a match
// against any is sufficient. The timestamp must be within tolerance of now.
func verifySvixSignature(secret, id, timestamp, signatureHeader string, body []byte, now time.Time) error {
	if id == "" || timestamp == "" || signatureHeader == "" {
		return errInvalidSignature
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return errInvalidSignature
	}
	if d := now.Unix() - ts; d > int64(resendWebhookTolerance/time.Second) || d < -int64(resendWebhookTolerance/time.Second) {
		return errInvalidSignature
	}

	raw := strings.TrimPrefix(secret, "whsec_")
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		// Some setups store the raw secret without base64; fall back to bytes.
		key = []byte(secret)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + timestamp + "."))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	for _, part := range strings.Fields(signatureHeader) {
		comma := strings.IndexByte(part, ',')
		if comma < 0 {
			continue
		}
		if hmac.Equal([]byte(part[comma+1:]), []byte(expected)) {
			return nil
		}
	}
	return errInvalidSignature
}
