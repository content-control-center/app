// Package schedule implements the shared "schedule a Post for
// publishing" operation (CON-78). It is the single source of truth used
// by the REST endpoint (POST /api/posts/:id/schedule), the Post
// Assistant's schedulePost tool, AND the existing PUT /api/posts/:id
// scheduling path — so all three can never drift on the rule that
// matters: how a post is routed to auto- vs manual-publish and how that
// decision is persisted transactionally with the Zernio enqueue.
//
// The instant a post is scheduled for is always stored as an absolute
// UTC time; the workspace timezone (CON-78) only affects how relative
// expressions are resolved and how the time is echoed — both of which
// happen in the caller, before this service runs, keeping the service
// deterministic (mirrors the CON-59 clone pattern).
package schedule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mikestefanello/backlite"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/jobs/queues"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/platforms"
	"github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/repository"
)

var (
	// ErrPostNotFound is returned when the post being scheduled does not exist.
	ErrPostNotFound = errors.New("post not found")
	// ErrScheduledAtRequired is returned when no scheduled_at was supplied.
	ErrScheduledAtRequired = errors.New("scheduled_at is required")
	// ErrScheduledAtInPast is returned when scheduled_at is not strictly in
	// the future at commit time (re-checked here because time passes between
	// the assistant's confirm turn and its commit turn — CON-78 §9).
	ErrScheduledAtInPast = errors.New("scheduled_at must be in the future")
	// ErrNoPlatform is returned when the post has no platform set, so neither
	// the allowlist nor pre-publish validation can be evaluated.
	ErrNoPlatform = errors.New("post has no platform set")
	// ErrNotSchedulable is returned when the post's status is not one that
	// can be scheduled (only ready_for_publish, or draft with AllowPromote).
	ErrNotSchedulable = errors.New("post is not in a schedulable state")
)

// Trigger identifies which entry point initiated a schedule (recorded in
// the audit log for attribution).
const (
	TriggerAPI       = "api"
	TriggerAssistant = "assistant"
)

// ValidationError wraps the CON-74 pre-publish failures encountered when
// auto-promoting a draft. The REST layer maps it to 422; the assistant
// declines and lists the per-platform reasons.
type ValidationError struct {
	Errors map[string][]platforms.ValidationError
}

func (e *ValidationError) Error() string {
	return "post failed pre-publish validation"
}

// Options controls a single schedule.
type Options struct {
	// ScheduledAt is the absolute instant to publish at. Required; relative
	// references are resolved by the caller. Stored as UTC.
	ScheduledAt time.Time
	// AllowPromote permits auto-promoting a draft (run CON-74 validation,
	// then Draft → ReadyForPublish) before scheduling.
	AllowPromote bool
	// Actor is the user id recorded on the audit entries. Required.
	Actor string
	// Trigger is TriggerAPI or TriggerAssistant.
	Trigger string
}

// Result is the outcome of a successful schedule.
type Result struct {
	// Post is the post after scheduling (status + scheduled_at applied).
	Post *models.Post
	// ScheduledAt is the absolute UTC instant the post was scheduled for.
	ScheduledAt time.Time
	// Status is the routed status: Scheduled (auto) or
	// ScheduledForManualPublish (not allowlisted).
	Status models.PostStatus
	// AutoPublish reports whether the platform was allowlisted (and a
	// Zernio submit was enqueued).
	AutoPublish bool
	// Promoted is true when a draft was auto-promoted to ready_for_publish
	// as part of this schedule.
	Promoted bool
}

// Service performs schedules. Construct with New.
type Service struct {
	db          *bun.DB
	posts       repository.PostRepository
	platforms   repository.PlatformRepository
	attachments repository.PostAttachmentRepository
	allowlist   repository.AutoPublishAllowlistRepository
	logs        repository.PostLogRepository
	jobs        *backlite.Client
	hub         eventhub.Hub

	// now is injectable so tests can pin "current time" for future-time
	// validation. nil → time.Now().UTC().
	now func() time.Time
}

// New wires a schedule Service. allowlist/jobs/logs/hub may be nil
// (degrades gracefully: nil allowlist → everything routes to manual
// publish; nil jobs → no enqueue; nil logs → no audit; nil hub → silent).
func New(
	db *bun.DB,
	posts repository.PostRepository,
	platformRepo repository.PlatformRepository,
	attachments repository.PostAttachmentRepository,
	allowlist repository.AutoPublishAllowlistRepository,
	logs repository.PostLogRepository,
	jobs *backlite.Client,
	hub eventhub.Hub,
) *Service {
	return &Service{
		db:          db,
		posts:       posts,
		platforms:   platformRepo,
		attachments: attachments,
		allowlist:   allowlist,
		logs:        logs,
		jobs:        jobs,
		hub:         hub,
	}
}

// SetClock overrides the time source (tests only).
func (s *Service) SetClock(fn func() time.Time) { s.now = fn }

func (s *Service) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now().UTC()
}

// Schedule schedules the post identified by postID for opts.ScheduledAt.
// It validates the post's state and the future time, optionally
// auto-promotes a draft (CON-74), routes to auto- vs manual-publish via
// the allowlist, and persists the status change, scheduled_at, audit
// entries, and (for auto-publish) the Zernio submit enqueue in a single
// transaction. Relative-time resolution is the caller's job — opts.
// ScheduledAt must already be absolute.
func (s *Service) Schedule(ctx context.Context, postID string, opts Options) (*Result, error) {
	if opts.ScheduledAt.IsZero() {
		return nil, ErrScheduledAtRequired
	}

	post, err := s.posts.GetByID(ctx, postID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPostNotFound
		}
		return nil, err
	}

	scheduledAt := opts.ScheduledAt.UTC()
	// Re-validate the future time here (not just in the handler): on the
	// assistant path several seconds pass between confirm and commit.
	if !scheduledAt.After(s.clock()) {
		return nil, ErrScheduledAtInPast
	}

	if post.PlatformID == "" {
		return nil, ErrNoPlatform
	}

	promoted := false
	var extraLogs []*models.PostLog

	switch post.Status {
	case models.PostStatusReadyForPublish:
		// Already publishable — schedule directly.
	case models.PostStatusDraft:
		if !opts.AllowPromote {
			return nil, fmt.Errorf("%w: %s (promotion not requested)", ErrNotSchedulable, post.Status)
		}
		platform, atts, lerr := s.loadForValidation(ctx, post)
		if lerr != nil {
			return nil, lerr
		}
		if errsByPlatform := s.validateForPublish(post, platform, atts); errsByPlatform != nil {
			return nil, &ValidationError{Errors: errsByPlatform}
		}
		promoted = true
		// Record the Draft → ReadyForPublish promote as its own audit edge
		// so the history reads correctly (validation passed, then promote).
		extraLogs = s.promoteLogs(post.ID, opts.Actor)
	default:
		return nil, fmt.Errorf("%w: %s", ErrNotSchedulable, post.Status)
	}

	// Route + persist from the (possibly just-promoted) ready_for_publish
	// state so every state-machine edge in the log is valid.
	autoPublish, target, supported, err := s.route(ctx, post.PlatformID)
	if err != nil {
		return nil, err
	}

	post.Status = target
	post.ScheduledAt = &scheduledAt
	post.UpdatedAt = s.clock()

	logs := append(extraLogs,
		s.decisionLog(post.ID, models.PostStatusReadyForPublish, target, post.PlatformID, supported, autoPublish, opts.Actor),
		s.transitionLog(post.ID, models.PostStatusReadyForPublish, target, "post scheduled", opts.Actor),
		s.scheduleActionLog(post.ID, scheduledAt, target, autoPublish, promoted, opts),
	)

	if err := s.persist(ctx, post, autoPublish, logs); err != nil {
		return nil, err
	}

	s.publishScheduled(post.ID, opts.Actor, scheduledAt, target, autoPublish, promoted)

	return &Result{
		Post:        post,
		ScheduledAt: scheduledAt,
		Status:      target,
		AutoPublish: autoPublish,
		Promoted:    promoted,
	}, nil
}

// RouteAndPersist is the thin entry used by the PUT /api/posts/:id
// scheduling path. The caller has already applied its full field set and
// set ScheduledAt on `post`; this routes the platform through the
// allowlist, sets the routed status, and writes the post row + the
// allowlist-decision + state-transition logs + (auto only) Zernio enqueue
// in one transaction. Returns the routed status string for the
// X-Auto-Publish-Decision response header. It deliberately does NOT
// re-validate state/time or emit the user_schedule action log — the PUT
// path keeps its historical, narrower behaviour; only the routing rule
// and the transactional write are shared.
func (s *Service) RouteAndPersist(ctx context.Context, post *models.Post, prevStatus models.PostStatus, actor string) (string, error) {
	autoPublish, target, supported, err := s.route(ctx, post.PlatformID)
	if err != nil {
		return "", err
	}
	post.Status = target
	post.UpdatedAt = s.clock()

	logs := []*models.PostLog{
		s.decisionLog(post.ID, prevStatus, target, post.PlatformID, supported, autoPublish, actor),
		s.transitionLog(post.ID, prevStatus, target, "status changed via PUT /api/posts/:id (schedule)", actor),
	}
	if err := s.persist(ctx, post, autoPublish, logs); err != nil {
		return "", err
	}
	return string(target), nil
}

// route reads the auto-publish allowlist for the post's platform and
// returns the resulting decision: autoPublish + the routed target status
// (Scheduled vs ScheduledForManualPublish) + the matched Zernio platform
// (nil when the platform isn't Zernio-supported). Pure read — no writes —
// so callers can build their audit log before opening the transaction.
func (s *Service) route(ctx context.Context, platformID string) (autoPublish bool, target models.PostStatus, supported *zernio.SupportedPlatform, err error) {
	supported = zernio.LookupSupportedBySqid(platformID)
	if supported != nil && s.allowlist != nil {
		ok, aerr := s.allowlist.Contains(ctx, supported.ZernioID)
		if aerr != nil {
			return false, "", nil, aerr
		}
		autoPublish = ok
	}
	target = models.PostStatusScheduledForManualPublish
	if autoPublish {
		target = models.PostStatusScheduled
	}
	return autoPublish, target, supported, nil
}

// persist writes the post row, every supplied log entry, and (when
// autoPublish) the Zernio submit enqueue in one transaction so a failure
// in any of them rolls the whole schedule back — no half-scheduled post,
// no enqueue without a status change (CON-78 §9).
func (s *Service) persist(ctx context.Context, post *models.Post, autoPublish bool, logs []*models.PostLog) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewUpdate().Model(post).WherePK().Exec(ctx); err != nil {
			return err
		}
		if s.logs != nil {
			for _, l := range logs {
				if l == nil {
					continue
				}
				if err := s.logs.AppendTx(ctx, tx, l); err != nil {
					return err
				}
			}
		}
		if autoPublish && s.jobs != nil {
			if _, err := s.jobs.Add(queues.SubmitPostTask{PostID: post.ID}).Tx(tx.Tx).Save(); err != nil {
				return err
			}
		}
		return nil
	})
}

// loadForValidation fetches the post's platform and attachments for the
// CON-74 promote gate. A missing platform row is treated as ErrNoPlatform.
func (s *Service) loadForValidation(ctx context.Context, post *models.Post) (*models.Platform, []models.PostAttachment, error) {
	platform := post.Platform
	if platform == nil || platform.ID != post.PlatformID {
		if s.platforms == nil {
			return nil, nil, ErrNoPlatform
		}
		p, err := s.platforms.GetByID(ctx, post.PlatformID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil, ErrNoPlatform
			}
			return nil, nil, err
		}
		platform = p
	}
	var atts []models.PostAttachment
	if s.attachments != nil {
		a, err := s.attachments.ListByPostID(ctx, post.ID)
		if err != nil {
			return nil, nil, err
		}
		atts = a
	}
	return platform, atts, nil
}

// validateForPublish runs the CON-74 readiness rules (attachment rules +
// per-post-type rules) for the promote gate. Returns nil when the post
// passes, or the populated per-platform error map when it does not.
func (s *Service) validateForPublish(post *models.Post, platform *models.Platform, atts []models.PostAttachment) map[string][]platforms.ValidationError {
	errsByPlatform := platforms.ValidateForPublish(atts, []*models.Platform{platform})
	if typeErrs := platforms.ValidatePostType(post, platform, atts); len(typeErrs) > 0 {
		errsByPlatform[platform.ID] = append(errsByPlatform[platform.ID], typeErrs...)
	}
	if !hasAnyErrors(errsByPlatform) {
		return nil
	}
	return errsByPlatform
}

func hasAnyErrors(m map[string][]platforms.ValidationError) bool {
	for _, v := range m {
		if len(v) > 0 {
			return true
		}
	}
	return false
}

func zernioName(s *zernio.SupportedPlatform) string {
	if s == nil {
		return ""
	}
	return s.ZernioID
}
