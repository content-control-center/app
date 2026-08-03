package queues

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/riverqueue/river"

	"github.com/ogen-app/ogen/src/email"
	"github.com/ogen-app/ogen/src/email/templates"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// SendEmailQueue is the single async send path for all mail (CON-154). Welcome
// (transactional, immediate) and the marketing drip (delayed via ScheduledAt)
// both flow through it; the recipient + suppression are resolved fresh at send
// time so an unsubscribe or email change is honoured without touching the
// already-scheduled jobs.
const SendEmailQueue = "send_email"

// SendEmailTask carries the addressing indirection (user + tenant) rather than
// a baked recipient, so the worker re-resolves the current address. EmailKind
// (note: the field is not Kind — that name is taken by the river.JobArgs method)
// drives suppression semantics + whether an unsubscribe footer is required.
type SendEmailTask struct {
	UserID         string           `json:"user_id"`
	TenantID       string           `json:"tenant_id"`
	TemplateKey    string           `json:"template_key"`
	EmailKind      models.EmailKind `json:"email_kind"`
	IdempotencyKey string           `json:"idempotency_key"`
}

// Kind implements river.JobArgs.
func (SendEmailTask) Kind() string { return SendEmailQueue }

// InsertOpts bounds retries and dedupes by args. The hard idempotency backstops
// are the Resend Idempotency-Key header + the email_logs unique index; ByArgs
// just stops an accidental duplicate enqueue from queueing twice.
func (SendEmailTask) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5, UniqueOpts: river.UniqueOpts{ByArgs: true}}
}

// SendEmailProcessor is the River worker for send_email.
type SendEmailProcessor struct {
	river.WorkerDefaults[SendEmailTask]
	Deps EmailDeps
}

// Work is the River entrypoint; it runs under the task's tenant scope so the
// recipient lookup is correctly isolated.
func (p *SendEmailProcessor) Work(ctx context.Context, job *river.Job[SendEmailTask]) error {
	ctx = WithJobRequestID(ctx, job.JobRow)
	return p.Process(ctx, job.Args)
}

// Timeout is the per-attempt context deadline.
func (p *SendEmailProcessor) Timeout(*river.Job[SendEmailTask]) time.Duration {
	return 30 * time.Second
}

func init() {
	register(func(w *river.Workers, d Deps) {
		river.AddWorker(w, &SendEmailProcessor{Deps: d.Email})
	})
}

// Process renders + sends one email. Only transient failures return an error
// (so River retries); every terminal outcome (skip, disabled, render failure,
// 4xx) is logged and returns nil.
func (p *SendEmailProcessor) Process(ctx context.Context, t SendEmailTask) error {
	const comp = "jobs.send_email"
	dep := p.Deps
	if t.TemplateKey == "" || t.UserID == "" {
		slog.WarnContext(ctx, "send_email skipped: missing args", logging.AttrComponent, comp)
		return nil
	}
	if dep.Users == nil || dep.Templates == nil {
		slog.WarnContext(ctx, "send_email skipped: deps not wired", logging.AttrComponent, comp)
		return nil
	}
	ctx = tenantctx.With(ctx, t.TenantID)

	// Resolve the recipient fresh: honours an email change since enqueue, and a
	// deleted user becomes a clean terminal skip.
	user, err := dep.Users.GetByIDWithTenant(ctx, t.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		slog.InfoContext(ctx, "send_email skipped: user gone", logging.AttrComponent, comp, "user", t.UserID)
		return nil
	} else if err != nil {
		return err // transient (DB)
	}
	toEmail := user.Email
	workspace := ""
	if user.Tenant != nil {
		workspace = user.Tenant.Name
	}

	logBase := models.EmailLog{
		TenantID:       t.TenantID,
		UserID:         t.UserID,
		TemplateID:     t.TemplateKey,
		Kind:           t.EmailKind,
		ToEmail:        toEmail,
		Provider:       models.ProviderResend,
		IdempotencyKey: t.IdempotencyKey,
	}

	// Sending disabled (no Resend key wired): record and succeed.
	if dep.Sender == nil {
		p.writeLog(ctx, logBase, models.EmailLogSkippedDisabled, "", "")
		slog.InfoContext(ctx, "send_email skipped: sending disabled", logging.AttrComponent, comp, "template", t.TemplateKey)
		return nil
	}

	// Send-time suppression gate (CON-154 D2).
	if dep.Suppressions != nil {
		suppressed, err := dep.Suppressions.IsSuppressed(ctx, toEmail, t.EmailKind)
		if err != nil {
			return err // transient (DB)
		}
		if suppressed {
			p.writeLog(ctx, logBase, models.EmailLogSkippedSuppressed, "", "")
			slog.InfoContext(ctx, "send_email skipped: suppressed", logging.AttrComponent, comp, "template", t.TemplateKey, "kind", string(t.EmailKind))
			return nil
		}
	}

	tmpl, err := dep.Templates.GetByKey(ctx, t.TemplateKey)
	if errors.Is(err, sql.ErrNoRows) {
		p.writeLog(ctx, logBase, models.EmailLogFailed, "", "template not found: "+t.TemplateKey)
		slog.WarnContext(ctx, "send_email failed: template not found", logging.AttrComponent, comp, "template", t.TemplateKey)
		return nil // terminal
	} else if err != nil {
		return err // transient (DB)
	}

	// Marketing mail must carry an unsubscribe affordance.
	headers := map[string]string{}
	unsubURL := ""
	if t.EmailKind == models.EmailKindMarketing {
		secret := ""
		if dep.LinkSecret != nil {
			secret, err = dep.LinkSecret(ctx)
			if err != nil {
				return err // transient (secret read)
			}
		}
		if secret == "" {
			p.writeLog(ctx, logBase, models.EmailLogFailed, "", "no link secret for unsubscribe")
			slog.WarnContext(ctx, "send_email failed: no unsubscribe secret", logging.AttrComponent, comp, "template", t.TemplateKey)
			return nil // terminal
		}
		unsubURL = dep.unsubscribeURL(secret, toEmail)
		headers["List-Unsubscribe"] = "<" + unsubURL + ">"
		headers["List-Unsubscribe-Post"] = "List-Unsubscribe=One-Click"
	}

	rendered, err := templates.Render(tmpl, templates.Data{
		Name:           user.Name,
		WorkspaceName:  workspace,
		AppURL:         dep.AppBaseURL,
		UnsubscribeURL: unsubURL,
	})
	if err != nil {
		p.writeLog(ctx, logBase, models.EmailLogFailed, "", "render: "+err.Error())
		slog.WarnContext(ctx, "send_email failed: render", logging.AttrComponent, comp, "template", t.TemplateKey, logging.AttrError, err)
		return nil // terminal
	}

	msgID, err := dep.Sender.Send(ctx, email.Message{
		From:           dep.From,
		ReplyTo:        dep.ReplyTo,
		To:             toEmail,
		Subject:        rendered.Subject,
		HTML:           rendered.HTML,
		Text:           rendered.Text,
		Headers:        headers,
		IdempotencyKey: t.IdempotencyKey,
	})
	if err != nil {
		if email.IsDisabled(err) {
			// No Resend key configured at send time (e.g. cleared after enqueue):
			// record as disabled, not failed — the subsystem is intentionally off.
			p.writeLog(ctx, logBase, models.EmailLogSkippedDisabled, "", "")
			slog.InfoContext(ctx, "send_email skipped: sending disabled", logging.AttrComponent, comp, "template", t.TemplateKey)
			return nil
		}
		if email.IsTransient(err) {
			slog.WarnContext(ctx, "send_email transient failure; will retry", logging.AttrComponent, comp, "template", t.TemplateKey, logging.AttrError, err)
			return err
		}
		p.writeLog(ctx, logBase, models.EmailLogFailed, "", err.Error())
		slog.WarnContext(ctx, "send_email terminal failure", logging.AttrComponent, comp, "template", t.TemplateKey, logging.AttrError, err)
		return nil // terminal
	}

	p.writeLog(ctx, logBase, models.EmailLogSent, msgID, "")
	slog.InfoContext(ctx, "send_email sent", logging.AttrComponent, comp, "template", t.TemplateKey, "kind", string(t.EmailKind))
	return nil
}

// writeLog appends one email_logs row (best-effort). The mail decision is
// already made by the time this runs, so a log failure — including a unique
// violation from a prior attempt that already logged this idempotency key — is
// warned and swallowed rather than failing the job (which would re-send).
func (p *SendEmailProcessor) writeLog(ctx context.Context, base models.EmailLog, status models.EmailLogStatus, providerMsgID, errMsg string) {
	if p.Deps.Logs == nil {
		return
	}
	id, err := models.NewID()
	if err != nil {
		slog.WarnContext(ctx, "email_log id gen failed", logging.AttrComponent, "jobs.send_email", logging.AttrError, err)
		return
	}
	row := base
	row.ID = id
	row.Status = status
	row.ProviderMessageID = providerMsgID
	row.Error = errMsg
	if err := p.Deps.Logs.Insert(ctx, &row); err != nil {
		slog.WarnContext(ctx, "email_log insert failed (best-effort)", logging.AttrComponent, "jobs.send_email", logging.AttrError, err)
	}
}
