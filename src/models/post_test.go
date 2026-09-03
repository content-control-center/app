package models

import "testing"

// The CON-130 edges: a scheduled post can be converted straight to
// manual publishing, and a manually-scheduled post can go straight to
// drafts — neither existed before and both are load-bearing for the
// convert-to-manual flow and channel-removal-to-drafts.
func TestCanTransitionCON130Edges(t *testing.T) {
	cases := []struct {
		from PostStatus
		to   PostStatus
		want bool
	}{
		// New CON-130 edges.
		{PostStatusScheduled, PostStatusScheduledForManualPublish, true},
		{PostStatusScheduledForManualPublish, PostStatusDraft, true},

		// Pre-existing edges still hold.
		{PostStatusScheduled, PostStatusReadyForPublish, true},
		{PostStatusScheduled, PostStatusDraft, true},
		{PostStatusReadyForPublish, PostStatusScheduled, true},
		{PostStatusScheduledForManualPublish, PostStatusPublished, true},

		// Still-illegal edges: manual publish can't jump back to auto
		// scheduled, and draft still can't leap straight to scheduled.
		{PostStatusScheduledForManualPublish, PostStatusScheduled, false},
		{PostStatusDraft, PostStatusScheduled, false},

		// Identity is always allowed.
		{PostStatusScheduled, PostStatusScheduled, true},
	}
	for _, tc := range cases {
		if got := tc.from.CanTransition(tc.to); got != tc.want {
			t.Errorf("CanTransition(%q -> %q) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

// CON-251: failed and not_published reopen straight to draft (the copy
// outside Ogen is gone, or never left, so an edit is the point). The
// pre-existing → ready_for_publish edges still hold, and neither
// submitted state gains a → draft shortcut here.
func TestCanTransitionCON251ReopenEdges(t *testing.T) {
	cases := []struct {
		from PostStatus
		to   PostStatus
		want bool
	}{
		// New CON-251 reopen edges.
		{PostStatusFailed, PostStatusDraft, true},
		{PostStatusNotPublished, PostStatusDraft, true},

		// Pre-existing reopen edges still hold.
		{PostStatusFailed, PostStatusReadyForPublish, true},
		{PostStatusNotPublished, PostStatusReadyForPublish, true},
		{PostStatusNotPublished, PostStatusScheduledForManualPublish, true},

		// The submitted states are NOT reopened to draft by this change —
		// their copy still lives outside Ogen (scheduled → draft was already
		// legal via cancel; published has no outgoing edge at all).
		{PostStatusPublished, PostStatusDraft, false},
	}
	for _, tc := range cases {
		if got := tc.from.CanTransition(tc.to); got != tc.want {
			t.Errorf("CanTransition(%q -> %q) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

// CON-251: IsSubmitted is true exactly for the two states that hold a copy
// outside Ogen — scheduled (Zernio) and published (the network) — and
// false for every editable state, including scheduled_for_manual_publishing
// (nobody holds it yet).
func TestIsSubmitted(t *testing.T) {
	submitted := map[PostStatus]bool{
		PostStatusScheduled: true,
		PostStatusPublished: true,
	}
	all := []PostStatus{
		PostStatusDraft, PostStatusReadyForPublish, PostStatusScheduled,
		PostStatusScheduledForManualPublish, PostStatusFailed,
		PostStatusPublished, PostStatusNotPublished,
	}
	for _, s := range all {
		if got := s.IsSubmitted(); got != submitted[s] {
			t.Errorf("IsSubmitted(%q) = %v, want %v", s, got, submitted[s])
		}
	}
}
