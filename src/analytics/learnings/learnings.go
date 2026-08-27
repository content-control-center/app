// Package learnings assembles the CON-239 "What we've learned" board — the
// all-time, slow-moving companion to the windowed dashboards (CON-237/238). It
// has three sections: a day-of-week × hour heatmap of where posts do best, a
// post-lifespan accrual curve (how long a post keeps earning), and deterministic
// structural "what works / what's fading" patterns. Build is a pure function
// over already-fetched data (per-post facts + per-post lifespan samples) so it
// is fully unit-testable without a database.
//
// Everything here is deterministic, no-AI: the patterns mine structural post
// attributes (media format, length, hashtags/links, timing, platform), not
// content semantics. Each section degrades independently to
// {insufficient_history:true} when there isn't enough data.
package learnings

import "time"

// Metric choices for the heatmap intensity + pattern mining.
const (
	MetricReach = "reach"
	MetricSaves = "saves"
)

const defaultTrendWindowDays = 90

// PostFact is one published post's metrics + structural attributes, already
// joined app-side (analytics metrics + main-DB post metadata).
type PostFact struct {
	PublishedAt   time.Time
	Platform      string
	Reach         int
	Saves         int
	MediaCount    int  // len(media_urls)
	IsVideo       bool // platform_post_type is a video/reel/short
	ContentLength int  // len(content)
	HashtagCount  int
	HasLink       bool
}

// LifespanPoint is one (post, age-hour) reach observation for the accrual curve.
type LifespanPoint struct {
	PostID   string
	AgeHours int
	Reach    int
}

// Inputs is everything Build needs.
type Inputs struct {
	Now             time.Time
	Since           *time.Time // nil = all-time
	TrendWindowDays int
	Metric          string
	Posts           []PostFact
	Lifespan        []LifespanPoint
	UpdatedAt       time.Time
}

// Scope echoes the resolved parameters + coverage counts.
type Scope struct {
	Since           *string `json:"since"`
	TrendWindowDays int     `json:"trend_window_days"`
	MeasuredPosts   int     `json:"measured_posts"`
	SettledPosts    int     `json:"settled_posts"`
	Metric          string  `json:"metric"`
}

// Response is the assembled board. Each section is non-nil; a section with too
// little data serialises as {"insufficient_history": true}.
type Response struct {
	Scope     Scope     `json:"scope"`
	UpdatedAt time.Time `json:"updated_at"`
	Heatmap   *Heatmap  `json:"heatmap"`
	Lifespan  *Lifespan `json:"lifespan"`
	Patterns  *Patterns `json:"patterns"`
}

// Build computes all three sections.
func Build(in Inputs) Response {
	metric := in.Metric
	if metric != MetricSaves {
		metric = MetricReach
	}
	trendDays := in.TrendWindowDays
	if trendDays <= 0 {
		trendDays = defaultTrendWindowDays
	}

	heat := buildHeatmap(in.Posts, metric)
	life := buildLifespan(in.Lifespan)
	pat := buildPatterns(in.Posts, metric, in.Now, trendDays)

	var sinceStr *string
	if in.Since != nil {
		s := in.Since.Format("2006-01-02")
		sinceStr = &s
	}

	return Response{
		Scope: Scope{
			Since:           sinceStr,
			TrendWindowDays: trendDays,
			MeasuredPosts:   len(in.Posts),
			SettledPosts:    life.SettledPosts,
			Metric:          metric,
		},
		UpdatedAt: in.UpdatedAt,
		Heatmap:   heat,
		Lifespan:  life,
		Patterns:  pat,
	}
}

// metricValue reads the chosen metric off a fact.
func metricValue(f PostFact, metric string) int {
	if metric == MetricSaves {
		return f.Saves
	}
	return f.Reach
}
