package post_quality

import "github.com/ogen-app/ogen/src/domain/models"

// ComposeScore applies a weight profile to the model's four sub-scores,
// stamps each dimension's Weight and Contribution into result in place,
// and returns the overall percentage. It is a pure function — no model,
// no IO — and is the single source of truth for the overall score, which
// the model never returns (CON-85 weighting contract).
//
//	overall = Σ weight[dim] × (score[dim] / 10), expressed as a percentage
//
// Each dimension's Contribution is weight × (score/10) × 100 — the
// percentage points it adds to the overall — so the four contributions
// sum to overall. The dimension's Weight is recorded too, leaving the
// persisted result self-describing.
//
// Scores are clamped to [0,10] defensively. validateOutput rejects
// out-of-range scores before composition runs, but clamping guarantees
// the overall stays in [0,100] no matter what reaches this function.
func ComposeScore(result *models.EvaluationResult, profile Profile) float64 {
	overall := stampDimension(&result.Correctness, profile.Correctness)
	overall += stampDimension(&result.Clarity, profile.Clarity)
	overall += stampDimension(&result.Engagement, profile.Engagement)
	overall += stampDimension(&result.Delivery, profile.Delivery)
	return overall
}

// stampDimension records the weight and weighted contribution on a single
// dimension and returns that contribution (in percentage points).
func stampDimension(dim *models.EvaluationDimension, weight float64) float64 {
	score := dim.Score
	switch {
	case score < 0:
		score = 0
	case score > 10:
		score = 10
	}
	contribution := weight * (float64(score) / 10) * 100
	dim.Weight = weight
	dim.Contribution = contribution
	return contribution
}
