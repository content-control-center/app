package post_quality

import (
	"math"
	"testing"

	"github.com/ogen-app/ogen/src/models"
)

const eps = 1e-9

func almostEqual(a, b float64) bool { return math.Abs(a-b) < eps }

func dims(c, cl, e, d int) models.EvaluationResult {
	return models.EvaluationResult{
		Correctness: models.EvaluationDimension{Score: c},
		Clarity:     models.EvaluationDimension{Score: cl},
		Engagement:  models.EvaluationDimension{Score: e},
		Delivery:    models.EvaluationDimension{Score: d},
	}
}

func TestComposeScore(t *testing.T) {
	text := defaultProfiles["text-post"]
	image := defaultProfiles["image-post"]

	tests := []struct {
		name    string
		result  models.EvaluationResult
		profile Profile
		want    float64
	}{
		{"all tens is 100 (text)", dims(10, 10, 10, 10), text, 100},
		{"all tens is 100 (image)", dims(10, 10, 10, 10), image, 100},
		{"all zeros is 0", dims(0, 0, 0, 0), text, 0},
		// text profile 0.30/0.30/0.20/0.20: 24 + 21 + 12 + 14 = 71
		{"mixed text", dims(8, 7, 6, 7), text, 71},
		// image profile 0.20/0.15/0.35/0.30 with all 5s: 5/10*100=50 of each weight => 50
		{"all fives is 50 (image)", dims(5, 5, 5, 5), image, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.result
			got := ComposeScore(&r, tt.profile)
			if !almostEqual(got, tt.want) {
				t.Errorf("overall = %v, want %v", got, tt.want)
			}
			// Invariant: the four contributions sum to the overall.
			sum := r.Correctness.Contribution + r.Clarity.Contribution +
				r.Engagement.Contribution + r.Delivery.Contribution
			if !almostEqual(sum, got) {
				t.Errorf("contributions sum = %v, want overall %v", sum, got)
			}
		})
	}
}

func TestComposeScoreStampsWeights(t *testing.T) {
	r := dims(8, 7, 6, 7)
	p := defaultProfiles["text-post"]
	ComposeScore(&r, p)

	if r.Correctness.Weight != p.Correctness ||
		r.Clarity.Weight != p.Clarity ||
		r.Engagement.Weight != p.Engagement ||
		r.Delivery.Weight != p.Delivery {
		t.Errorf("weights not stamped: got C=%v Cl=%v E=%v D=%v",
			r.Correctness.Weight, r.Clarity.Weight, r.Engagement.Weight, r.Delivery.Weight)
	}
	// Correctness: 0.30 * 8/10 * 100 = 24
	if !almostEqual(r.Correctness.Contribution, 24) {
		t.Errorf("correctness contribution = %v, want 24", r.Correctness.Contribution)
	}
}

func TestComposeScoreClampsOutOfRange(t *testing.T) {
	// Score above 10 is treated as 10, below 0 as 0; overall stays in range.
	r := dims(15, -3, 10, 0)
	got := ComposeScore(&r, defaultProfiles["text-post"])
	// clamped to (10, 0, 10, 0): 0.30*100 + 0 + 0.20*100 + 0 = 50
	if !almostEqual(got, 50) {
		t.Errorf("clamped overall = %v, want 50", got)
	}
	if got < 0 || got > 100 {
		t.Errorf("overall %v out of [0,100]", got)
	}
}
