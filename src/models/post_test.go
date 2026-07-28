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
