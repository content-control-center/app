package learnings

import (
	"math"
	"strings"
)

func round2(f float64) float64 { return math.Round(f*100) / 100 }
func round4(f float64) float64 { return math.Round(f*10000) / 10000 }

// titleCasePlatform renders a platform key for display ("linkedin" → "LinkedIn").
func titleCasePlatform(p string) string {
	switch strings.ToLower(p) {
	case "linkedin":
		return "LinkedIn"
	case "instagram":
		return "Instagram"
	case "facebook":
		return "Facebook"
	case "youtube":
		return "YouTube"
	case "twitter", "x":
		return "X"
	case "threads":
		return "Threads"
	case "tiktok":
		return "TikTok"
	}
	if p == "" {
		return p
	}
	return strings.ToUpper(p[:1]) + p[1:]
}

// fractionWord renders a ratio < 1 as a friendly fraction ("two-thirds").
func fractionWord(r float64) string {
	switch {
	case r <= 0.30:
		return "under a third"
	case r <= 0.42:
		return "about a third"
	case r <= 0.58:
		return "about half"
	case r <= 0.70:
		return "about two-thirds"
	case r <= 0.80:
		return "about three-quarters"
	default:
		return "most"
	}
}

func metricNoun(metric string) string {
	if metric == MetricSaves {
		return "saves"
	}
	return "reach"
}
