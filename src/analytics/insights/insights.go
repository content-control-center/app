// Package insights is the shared, deterministic (no-AI) rule engine behind the
// analytics dashboards' text callouts (CON-237, reused by CON-238/239). Each
// rule is a pure predicate over already-computed window aggregates that emits a
// templated Insight; the engine orders by priority and caps the result. No I/O,
// no model inference — just basic data analysis over the numbers.
package insights

import "fmt"

// Severity tags an insight for the UI.
type Severity string

const (
	Info Severity = "info"
	Note Severity = "note"
)

// Insight is one rendered callout. Note is an optional secondary line.
type Insight struct {
	ID       string   `json:"id"`
	Severity Severity `json:"severity"`
	Text     string   `json:"text"`
	Note     string   `json:"note,omitempty"`
}

// Input is the aggregate snapshot the overview rules read: current-window and
// previous-window totals for each headline metric plus a couple of derived
// signals. Rates are fractions (0.05 = 5%).
type Input struct {
	ReachCur, ReachPrev               int
	InteractionsCur, InteractionsPrev int
	EngRateCur, EngRatePrev           float64
	FollowersCur, FollowersPrev       int
	PostsCur, PostsPrev               int

	// PeakBucketLabel is the busiest bucket (by reach added) and PeakBucketShare
	// its share of the window's reach (0..1). FollowerStreak is the count of
	// trailing consecutive buckets of follower growth.
	PeakBucketLabel string
	PeakBucketShare float64
	FollowerStreak  int
}

// maxInsights caps how many callouts the overview surfaces.
const maxInsights = 3

// up reports whether cur exceeds prev by more than a small relative threshold.
func up(cur, prev int) bool   { return prev > 0 && float64(cur-prev)/float64(prev) > 0.02 }
func down(cur, prev int) bool { return prev > 0 && float64(cur-prev)/float64(prev) < -0.02 }

// Build runs the overview rule catalog and returns up to maxInsights callouts,
// highest-priority first.
func Build(in Input) []Insight {
	var out []Insight

	// 1. rate_vs_reach mechanic — reach and engagement rate moved opposite ways.
	if up(in.ReachCur, in.ReachPrev) && in.EngRateCur < in.EngRatePrev {
		tail := "Interactions themselves are up."
		if !up(in.InteractionsCur, in.InteractionsPrev) {
			tail = "Interactions did not keep pace."
		}
		out = append(out, Insight{
			ID:       "rate_vs_reach",
			Severity: Info,
			Text:     "Engagement rate slipped while reach rose — you reached more people who were less inclined to react. " + tail,
			Note:     "Rate is interactions ÷ reach, so a reach spike depresses it mechanically.",
		})
	}

	// 2. reinforcing vs diverging — reach and interactions agreement.
	switch {
	case up(in.ReachCur, in.ReachPrev) && up(in.InteractionsCur, in.InteractionsPrev):
		out = append(out, Insight{
			ID:       "reinforcing",
			Severity: Info,
			Text:     "Reach and interactions are both up — healthy, broad-based growth this period.",
		})
	case down(in.ReachCur, in.ReachPrev) && down(in.InteractionsCur, in.InteractionsPrev):
		out = append(out, Insight{
			ID:       "reinforcing",
			Severity: Note,
			Text:     "Reach and interactions are both down from the previous stretch.",
		})
	}

	// 3. cadence -> output — did posting more move reach?
	if up(in.PostsCur, in.PostsPrev) && up(in.ReachCur, in.ReachPrev) {
		out = append(out, Insight{
			ID:       "cadence_output",
			Severity: Info,
			Text:     "You posted more and reached more — output and reach are moving together.",
		})
	} else if down(in.PostsCur, in.PostsPrev) && up(in.ReachCur, in.ReachPrev) {
		out = append(out, Insight{
			ID:       "cadence_output",
			Severity: Info,
			Text:     "Reach rose even though you published fewer posts — the posts you ran travelled further.",
		})
	}

	// 4. follower streak.
	if in.FollowerStreak >= 3 {
		out = append(out, Insight{
			ID:       "follower_streak",
			Severity: Info,
			Text:     fmt.Sprintf("Followers have grown %d buckets in a row.", in.FollowerStreak),
		})
	}

	// 5. peak bucket concentration.
	if in.PeakBucketShare >= 0.4 && in.PeakBucketLabel != "" {
		out = append(out, Insight{
			ID:       "peak_bucket",
			Severity: Note,
			Text:     fmt.Sprintf("Most of the movement landed on %s (%.0f%% of the window's reach).", in.PeakBucketLabel, in.PeakBucketShare*100),
		})
	}

	if len(out) > maxInsights {
		out = out[:maxInsights]
	}
	return out
}
