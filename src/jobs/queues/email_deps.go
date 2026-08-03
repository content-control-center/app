package queues

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/ogen-app/ogen/src/email"
	"github.com/ogen-app/ogen/src/repository"
)

// EmailDeps bundles everything the send_email + cleanup_email_logs workers need
// (CON-154). Constructed once at boot in server.go. A nil Sender means no
// Resend key is wired, so the send job degrades to skipped_disabled rather than
// erroring — mirroring the nil-client pattern used for PDF/Zernio.
type EmailDeps struct {
	Sender       email.Sender
	Templates    repository.EmailTemplateRepository
	Suppressions repository.EmailSuppressionRepository
	Logs         repository.EmailLogRepository
	Users        repository.UserRepository

	// From / ReplyTo are the message envelope; AppBaseURL builds absolute links
	// (unsubscribe, CTAs) in rendered mail.
	From       string
	ReplyTo    string
	AppBaseURL string

	// LinkSecret resolves the HMAC key that signs unsubscribe links, per call
	// (so a rotation takes effect with no restart). A read error is transient;
	// an empty value means marketing mail can't build an unsubscribe link.
	LinkSecret func(ctx context.Context) (string, error)

	// Retention is the cleanup_email_logs window; 0 disables the sweep.
	Retention time.Duration
}

// unsubscribeURL builds the absolute one-click unsubscribe link for toEmail,
// signed with secret. The address is normalised so the token round-trips
// exactly what the suppression list keys on.
func (d EmailDeps) unsubscribeURL(secret, toEmail string) string {
	token := email.SignUnsubscribe(secret, repository.NormalizeEmail(toEmail), time.Now().UTC())
	base := strings.TrimRight(d.AppBaseURL, "/")
	return base + "/api/email/unsubscribe?token=" + url.QueryEscape(token)
}
