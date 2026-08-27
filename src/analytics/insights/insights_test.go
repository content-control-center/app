package insights

import "testing"

func has(ins []Insight, id string) bool {
	for _, i := range ins {
		if i.ID == id {
			return true
		}
	}
	return false
}

func TestRateVsReachFires(t *testing.T) {
	// reach up, engagement rate down, interactions up.
	in := Input{
		ReachCur: 1000, ReachPrev: 500,
		InteractionsCur: 60, InteractionsPrev: 50,
		EngRateCur: 0.06, EngRatePrev: 0.10,
	}
	got := Build(in)
	if !has(got, "rate_vs_reach") {
		t.Fatalf("expected rate_vs_reach, got %+v", got)
	}
}

func TestReinforcingGrowth(t *testing.T) {
	in := Input{
		ReachCur: 1000, ReachPrev: 500,
		InteractionsCur: 100, InteractionsPrev: 50,
		EngRateCur: 0.10, EngRatePrev: 0.10, // equal → rate_vs_reach does not fire
	}
	got := Build(in)
	if !has(got, "reinforcing") {
		t.Fatalf("expected reinforcing, got %+v", got)
	}
	if has(got, "rate_vs_reach") {
		t.Fatalf("rate_vs_reach should not fire when rate is flat")
	}
}

func TestCapAndOrder(t *testing.T) {
	// Trigger many rules; expect a cap of 3, highest priority first.
	in := Input{
		ReachCur: 1000, ReachPrev: 500,
		InteractionsCur: 60, InteractionsPrev: 50,
		EngRateCur: 0.06, EngRatePrev: 0.10, // rate_vs_reach
		PostsCur: 20, PostsPrev: 10, // cadence_output
		FollowerStreak:  5,                                  // follower_streak
		PeakBucketLabel: "2026-08-12", PeakBucketShare: 0.7, // peak_bucket
	}
	got := Build(in)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (capped)", len(got))
	}
	if got[0].ID != "rate_vs_reach" {
		t.Fatalf("first = %s, want rate_vs_reach", got[0].ID)
	}
}

func TestEmptyWhenFlat(t *testing.T) {
	got := Build(Input{ReachCur: 100, ReachPrev: 100, InteractionsCur: 10, InteractionsPrev: 10})
	if len(got) != 0 {
		t.Fatalf("expected no insights on flat input, got %+v", got)
	}
}
