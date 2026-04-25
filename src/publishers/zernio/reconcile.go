package zernio

import (
	"time"

	"github.com/content-control-center/app/src/models"
)

// ReconcileChange is the transition observed for one ID between the
// previous local state and the new remote state. The worker uses these
// to decide which eventhub events to publish (attached / updated /
// disconnected / revived).
type ReconcileChange string

const (
	ChangeAttached     ReconcileChange = "attached"
	ChangeUpdated      ReconcileChange = "updated"
	ChangeDisconnected ReconcileChange = "disconnected"
	ChangeRevived      ReconcileChange = "revived"
)

// AccountChange pairs a SocialAccount row with the kind of transition
// the reconciler observed.
type AccountChange struct {
	Change  ReconcileChange
	Account models.SocialAccount
}

// ReconcilePlan is the result of comparing the remote and local sets.
//
//   - Upserts is the union of Inserts, Updates, and Revives — i.e. the
//     rows the host should write. Each row's LastSyncedAt is set to
//     the same `now` instant by Reconcile.
//   - SoftDeleteIDs are local active rows missing from remote.
//   - Changes describes the transitions for observability so the
//     worker can publish per-account events.
//
// The worker passes Upserts + SoftDeleteIDs to ApplyPlan inside one
// transaction, then publishes events keyed off Changes after commit.
type ReconcilePlan struct {
	Upserts       []models.SocialAccount
	SoftDeleteIDs []string
	Changes       []AccountChange
}

// Reconcile diffs remote (what Zernio reports today) against local
// (every row this profile has, including soft-deleted) and returns
// the plan. It is a pure function — no I/O — so tests can drive it
// without a stub HTTP server or a database.
//
// Decisions:
//
//   - remote ID not in local                   → attached (insert)
//   - remote ID in local with deleted_at != nil → revived (un-soft-delete)
//   - remote ID in local active, fields differ → updated
//   - remote ID in local active, fields equal  → no-op
//   - local active ID not in remote            → disconnected (soft-delete)
//
// "Fields differ" compares only the user-visible attributes the ticket
// calls out — username, display_name, is_active, avatar_url. raw_json
// is always refreshed even when no surface field changed, so Zernio
// metadata stays current.
func Reconcile(remote []Account, local []models.SocialAccount, profileID string, now time.Time) ReconcilePlan {
	localByID := make(map[string]models.SocialAccount, len(local))
	for _, l := range local {
		localByID[l.ID] = l
	}
	remoteByID := make(map[string]struct{}, len(remote))

	plan := ReconcilePlan{}
	for _, r := range remote {
		remoteByID[r.ID] = struct{}{}
		row := accountToRow(r, profileID, now)
		existing, ok := localByID[r.ID]
		switch {
		case !ok:
			plan.Upserts = append(plan.Upserts, row)
			plan.Changes = append(plan.Changes, AccountChange{Change: ChangeAttached, Account: row})
		case existing.DeletedAt != nil:
			plan.Upserts = append(plan.Upserts, row)
			plan.Changes = append(plan.Changes, AccountChange{Change: ChangeRevived, Account: row})
		case fieldsDiffer(existing, row):
			plan.Upserts = append(plan.Upserts, row)
			plan.Changes = append(plan.Changes, AccountChange{Change: ChangeUpdated, Account: row})
		default:
			// No surface change. We still want raw_json + last_synced_at
			// refreshed so the row reflects the most recent observation,
			// so issue an upsert without an event.
			plan.Upserts = append(plan.Upserts, row)
		}
	}
	for _, l := range local {
		if l.DeletedAt != nil {
			continue
		}
		if _, ok := remoteByID[l.ID]; ok {
			continue
		}
		plan.SoftDeleteIDs = append(plan.SoftDeleteIDs, l.ID)
		plan.Changes = append(plan.Changes, AccountChange{Change: ChangeDisconnected, Account: l})
	}
	return plan
}

// accountToRow projects a Zernio Account into the local row shape.
// ConnectedAt is sourced from the remote's CreatedAt so the local row
// retains a stable "first seen" timestamp across syncs.
func accountToRow(a Account, profileID string, now time.Time) models.SocialAccount {
	raw := string(a.Raw)
	if raw == "" {
		raw = "{}"
	}
	connectedAt := a.CreatedAt
	if connectedAt.IsZero() {
		connectedAt = now
	}
	return models.SocialAccount{
		ID:           a.ID,
		Platform:     a.Platform,
		ProfileID:    profileID,
		Username:     a.Username,
		DisplayName:  a.DisplayName,
		AvatarURL:    a.AvatarURL,
		IsActive:     a.IsActive,
		RawJSON:      raw,
		ConnectedAt:  connectedAt,
		LastSyncedAt: now,
	}
}

// fieldsDiffer reports whether the user-visible fields changed between
// the existing local row and the freshly-projected row. raw_json is
// excluded — it always refreshes — and timestamps are excluded
// because they always advance.
func fieldsDiffer(local, remote models.SocialAccount) bool {
	return local.Username != remote.Username ||
		local.DisplayName != remote.DisplayName ||
		local.AvatarURL != remote.AvatarURL ||
		local.IsActive != remote.IsActive ||
		local.Platform != remote.Platform
}
