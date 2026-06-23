package zernio

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// SyncIntervalFloor is the hard lower bound the worker enforces in
// code regardless of the configured ZERNIO_SYNC_INTERVAL — a runaway
// env value can't melt Zernio's API.
const SyncIntervalFloor = 10 * time.Second

// fastSyncIntervalFloor is the hard lower bound for the adaptive fast
// cadence that activates after a connect-link issuance.
const fastSyncIntervalFloor = 5 * time.Second

// rateLimitBackoffCap caps the doubled interval after a 429 from
// Zernio (per the ticket: "double the next interval, capped at 5 min").
const rateLimitBackoffCap = 5 * time.Minute

// SyncStatusOK / SyncStatusError are the documented values written
// to the zernio.last_sync_status setting.
const (
	SyncStatusOK    = "ok"
	syncErrorPrefix = "error: "
)

// EventTopicSync is the event topic for sync-wide events (success /
// failure). Per-account events use a per-id topic — see eventTopicForAccount.
const EventTopicSync = "zernio:sync"

// EventTypeAttached / EventTypeAttachFailed / EventTypeUpdated /
// EventTypeDisconnected / EventTypeRevived are the eventhub event
// types published per account. The names align with ReconcileChange
// values plus the explicit "attach_failed" signal the ticket asks
// for on persistence failure.
const (
	EventTypeAttached     = "zernio.account.attached"
	EventTypeAttachFailed = "zernio.account.attach_failed"
	EventTypeUpdated      = "zernio.account.updated"
	EventTypeDisconnected = "zernio.account.disconnected"
	EventTypeRevived      = "zernio.account.revived"
	EventTypeSyncOK       = "zernio.sync.ok"
	EventTypeSyncFailed   = "zernio.sync.failed"
)

// Worker periodically reconciles Zernio's view of accounts against
// the local social_accounts table. Started by the host once at boot
// and stopped via Stop(). The loop:
//
//  1. Wait until the integration is in StateOK and a profile_id is
//     known (bootstrap may still be in flight).
//  2. Tick: list remote → list local → reconcile → ApplyPlan →
//     publish per-change events → write last_sync_at / last_sync_status.
//  3. Compute next interval honouring (a) ZernioSyncInterval, (b)
//     the fast-cadence window from BumpFastUntil, (c) rate-limit
//     backoff, (d) the SyncIntervalFloor hard cap.
//
// Errors are non-fatal except 401 from Zernio, which transitions the
// integration to StateDisabled and exits the loop until restart or an
// admin /profile/repair.
type Worker struct {
	integ        *Integration
	accounts     repository.SocialAccountRepository
	settings     SettingsStore
	hub          eventhub.Hub
	bootstrapper *Bootstrapper

	interval     time.Duration
	fastInterval time.Duration

	trigger chan struct{}
	done    chan struct{}

	// rateLimitUntil is set by tick() when Zernio returns 429; the
	// next iteration sleeps until at least this instant.
	rateLimitUntil time.Time
}

func NewWorker(
	integ *Integration,
	accounts repository.SocialAccountRepository,
	settings SettingsStore,
	hub eventhub.Hub,
	bootstrapper *Bootstrapper,
	interval, fastInterval time.Duration,
) *Worker {
	if interval < SyncIntervalFloor {
		interval = SyncIntervalFloor
	}
	if fastInterval < fastSyncIntervalFloor {
		fastInterval = fastSyncIntervalFloor
	}
	return &Worker{
		integ:        integ,
		accounts:     accounts,
		settings:     settings,
		hub:          hub,
		bootstrapper: bootstrapper,
		interval:     interval,
		fastInterval: fastInterval,
		trigger:      make(chan struct{}, 1),
		done:         make(chan struct{}),
	}
}

// Run blocks until ctx is cancelled, ticking on the configured
// cadence. Safe to call from a goroutine.
func (w *Worker) Run(ctx context.Context) {
	defer close(w.done)
	log.Printf("zernio: sync worker started (interval=%s fast=%s)", w.interval, w.fastInterval)
	for {
		select {
		case <-ctx.Done():
			log.Printf("zernio: sync worker stopped")
			return
		default:
		}

		if !w.shouldTick() {
			// Bootstrap not done yet, or integration disabled — wait
			// out one full interval before checking again.
			if !w.sleep(ctx, w.interval) {
				return
			}
			continue
		}

		if err := w.tick(ctx); err != nil {
			if IsStatus(err, http.StatusUnauthorized) {
				log.Printf("zernio: 401 from Zernio — disabling integration")
				w.integ.SetState(StateDisabled)
				return
			}
			log.Printf("zernio: tick failed: %v", err)
		}

		if !w.sleep(ctx, w.nextInterval()) {
			return
		}
	}
}

// Done returns a channel closed when Run returns. Used by the host's
// shutdown hook to wait up to 2s for a graceful exit.
func (w *Worker) Done() <-chan struct{} { return w.done }

// TriggerNow asks the worker to run a tick at its earliest opportunity.
// Multiple calls coalesce — the channel is size 1 and a non-blocking
// send is dropped when one is already pending.
func (w *Worker) TriggerNow() {
	select {
	case w.trigger <- struct{}{}:
	default:
	}
}

// shouldTick reports whether a tick is worth attempting now. Avoids
// spamming Zernio when bootstrap hasn't populated a profile ID yet.
func (w *Worker) shouldTick() bool {
	if !w.integ.Enabled() || w.integ.State() == StateDisabled {
		return false
	}
	if time.Now().Before(w.rateLimitUntil) {
		return false
	}
	id, _, err := w.settings.Get(context.Background(), SettingProfileID)
	if err != nil || id == "" {
		return false
	}
	return true
}

// nextInterval returns the delay before the next tick, honouring fast
// cadence and any active rate-limit backoff. The floor still applies.
func (w *Worker) nextInterval() time.Duration {
	now := time.Now()
	if rl := w.rateLimitUntil; rl.After(now) {
		return rl.Sub(now)
	}
	d := w.interval
	if now.Before(w.integ.FastUntil()) {
		d = w.fastInterval
	}
	if d < SyncIntervalFloor {
		// Fast interval is allowed below SyncIntervalFloor (it has
		// its own 5s floor enforced at construction); only the
		// regular interval is bounded by SyncIntervalFloor.
		if d < fastSyncIntervalFloor {
			d = fastSyncIntervalFloor
		}
	}
	return d
}

// sleep blocks for d, returning false when ctx fires (so the caller
// can exit promptly) or when triggered (which short-circuits the wait).
func (w *Worker) sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	case <-w.trigger:
		return true
	}
}

// tick executes one reconciliation pass. Returns the underlying error
// so the loop can react to 401 vs 429 vs everything else.
//
// Log lines use the documented zernio.* key=value fields so a future
// structured-logging migration can ingest them without a regex pass.
func (w *Worker) tick(ctx context.Context) error {
	tickStart := time.Now()

	profileID, _, err := w.settings.Get(ctx, SettingProfileID)
	if err != nil {
		w.recordSyncStatus(ctx, err)
		return err
	}

	remote, err := w.integ.Client.ListAccounts(ctx, profileID)
	if err != nil {
		if IsStatus(err, http.StatusTooManyRequests) {
			w.handleRateLimit()
		}
		w.recordSyncStatus(ctx, err)
		w.publishSyncEvent(ctx, EventTypeSyncFailed, fmt.Sprintf("list_accounts: %v", err))
		log.Printf("zernio.sync failed zernio.profile_id=%s zernio.sync_tick_ms=%d error=%q",
			profileID, time.Since(tickStart).Milliseconds(), err.Error())
		return err
	}

	local, err := w.accounts.ListAll(ctx, profileID)
	if err != nil {
		w.recordSyncStatus(ctx, err)
		return err
	}

	now := time.Now().UTC()
	plan := Reconcile(remote, local, profileID, now)

	if err := w.accounts.ApplyPlan(ctx, plan.Upserts, plan.SoftDeleteIDs, now); err != nil {
		w.recordSyncStatus(ctx, err)
		for _, ch := range plan.Changes {
			if ch.Change == ChangeAttached {
				w.publishAccountEvent(ctx, EventTypeAttachFailed, ch.Account, fmt.Sprintf("apply_plan: %v", err))
			}
		}
		log.Printf("zernio.sync failed zernio.profile_id=%s zernio.sync_tick_ms=%d error=%q",
			profileID, time.Since(tickStart).Milliseconds(), err.Error())
		return err
	}

	var added, updated, removed int
	for _, ch := range plan.Changes {
		switch ch.Change {
		case ChangeAttached:
			added++
			w.publishAccountEvent(ctx, EventTypeAttached, ch.Account, "")
			log.Printf("zernio.account.attached zernio.profile_id=%s zernio.platform=%s zernio.account_id=%s",
				profileID, ch.Account.Platform, ch.Account.ID)
		case ChangeUpdated:
			updated++
			w.publishAccountEvent(ctx, EventTypeUpdated, ch.Account, "")
		case ChangeDisconnected:
			removed++
			w.publishAccountEvent(ctx, EventTypeDisconnected, ch.Account, "")
			log.Printf("zernio.account.disconnected zernio.profile_id=%s zernio.platform=%s zernio.account_id=%s",
				profileID, ch.Account.Platform, ch.Account.ID)
		case ChangeRevived:
			added++
			w.publishAccountEvent(ctx, EventTypeRevived, ch.Account, "")
		}
	}

	w.refreshProfileMeta(ctx, profileID)

	w.recordSyncStatus(ctx, nil)
	w.publishSyncEvent(ctx, EventTypeSyncOK, fmt.Sprintf("upserts=%d soft_deletes=%d", len(plan.Upserts), len(plan.SoftDeleteIDs)))
	log.Printf("zernio.sync ok zernio.profile_id=%s zernio.sync_tick_ms=%d zernio.accounts_added=%d zernio.accounts_updated=%d zernio.accounts_removed=%d",
		profileID, time.Since(tickStart).Milliseconds(), added, updated, removed)
	return nil
}

// handleRateLimit doubles the next interval, capped at the documented 5m.
func (w *Worker) handleRateLimit() {
	delay := w.interval * 2
	if delay > rateLimitBackoffCap {
		delay = rateLimitBackoffCap
	}
	w.rateLimitUntil = time.Now().Add(delay)
	log.Printf("zernio: rate-limited; backing off %s", delay)
}

// recordSyncStatus writes the documented last_sync_at / last_sync_status
// settings. Errors here are logged but not propagated — a status-write
// failure shouldn't suppress the underlying tick error.
func (w *Worker) recordSyncStatus(ctx context.Context, syncErr error) {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := w.settings.Set(ctx, SettingLastSyncAt, now); err != nil {
		log.Printf("zernio: write %s: %v", SettingLastSyncAt, err)
	}
	status := SyncStatusOK
	if syncErr != nil {
		status = syncErrorPrefix + truncate(syncErr.Error(), 200)
	}
	if err := w.settings.Set(ctx, SettingLastSyncStatus, status); err != nil {
		log.Printf("zernio: write %s: %v", SettingLastSyncStatus, err)
	}
}

// refreshProfileMeta re-reads the profile from Zernio and updates the
// settings cache. Best-effort: failure is logged and swallowed.
func (w *Worker) refreshProfileMeta(ctx context.Context, profileID string) {
	profile, err := w.integ.Client.GetProfile(ctx, profileID)
	if err != nil {
		log.Printf("zernio: refresh profile meta: %v", err)
		return
	}
	raw := profile.Raw
	if len(raw) == 0 {
		var mErr error
		if raw, mErr = json.Marshal(profile); mErr != nil {
			log.Printf("zernio: marshal profile meta: %v", mErr)
			return
		}
	}
	if err := w.settings.Set(ctx, SettingProfileMeta, string(raw)); err != nil {
		log.Printf("zernio: write %s: %v", SettingProfileMeta, err)
	}
	if err := w.settings.Set(ctx, SettingProfileName, profile.Name); err != nil {
		log.Printf("zernio: write %s: %v", SettingProfileName, err)
	}
}

// publishAccountEvent fires an eventhub event for one account change.
// Topic shape matches the in-tree convention (entity:<kind>:<id>).
func (w *Worker) publishAccountEvent(ctx context.Context, eventType string, account models.SocialAccount, errMsg string) {
	topic := fmt.Sprintf("entity:zernio_account:%s", account.ID)
	payload := map[string]any{
		"id":          account.ID,
		"platform":    account.Platform,
		"username":    account.Username,
		"displayName": account.DisplayName,
		"isActive":    account.IsActive,
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	if err := w.hub.Publish(ctx, eventhub.Event{
		Topic:    topic,
		TenantID: account.TenantID,
		Type:     eventType,
		Payload:  payload,
	}); err != nil {
		log.Printf("zernio: publish event %s: %v", eventType, err)
	}
}

// publishSyncEvent fires the broad sync.ok / sync.failed signal.
func (w *Worker) publishSyncEvent(ctx context.Context, eventType, summary string) {
	if err := w.hub.Publish(ctx, eventhub.Event{
		Topic:   EventTopicSync,
		Type:    eventType,
		Payload: map[string]any{"summary": summary},
	}); err != nil {
		log.Printf("zernio: publish event %s: %v", eventType, err)
	}
}

// truncate caps s at n characters with an ellipsis when needed. Used
// to keep last_sync_status from blowing up the settings row.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
