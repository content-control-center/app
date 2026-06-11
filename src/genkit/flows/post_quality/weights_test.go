package post_quality

import (
	"testing"

	"github.com/ogen-app/ogen/src/models"
)

func TestDefaultProfilesSumToOne(t *testing.T) {
	// A mistyped weight table silently skews every score, so guard the
	// "sums to 100%" contract for every built-in profile and the default.
	for slug, p := range defaultProfiles {
		if !almostEqual(p.Sum(), 1.0) {
			t.Errorf("profile %q sums to %v, want 1.0", slug, p.Sum())
		}
	}
	if !almostEqual(defaultTextProfile.Sum(), 1.0) {
		t.Errorf("default profile sums to %v, want 1.0", defaultTextProfile.Sum())
	}
}

func TestWeightsFor(t *testing.T) {
	w := DefaultWeights()

	if got := w.For("image-post"); got != defaultProfiles["image-post"] {
		t.Errorf("For(image-post) = %+v, want %+v", got, defaultProfiles["image-post"])
	}
	// Unknown slug and empty slug both fall back to the default.
	if got := w.For("reel"); got != defaultTextProfile {
		t.Errorf("For(reel) = %+v, want default %+v", got, defaultTextProfile)
	}
	if got := w.For(""); got != defaultTextProfile {
		t.Errorf("For(empty) = %+v, want default %+v", got, defaultTextProfile)
	}
}

func TestNewWeightsCopiesInput(t *testing.T) {
	src := map[string]Profile{"text-post": {Correctness: 1}}
	w := NewWeights(src, defaultTextProfile)
	// Mutating the caller's map after construction must not affect Weights.
	src["text-post"] = Profile{Engagement: 1}
	if got := w.For("text-post"); got.Correctness != 1 {
		t.Errorf("Weights did not copy input map: got %+v", got)
	}
}

func TestWeightsFromJSON(t *testing.T) {
	// Empty → built-in defaults.
	w, err := WeightsFromJSON("")
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if w.For("text-post") != defaultProfiles["text-post"] {
		t.Errorf("empty did not yield built-in defaults")
	}

	// Override extends defaults with a new slug and replaces the default.
	js := `{"profiles":{"reel":{"correctness":0.2,"clarity":0.15,"engagement":0.4,"delivery":0.25}},
	        "default":{"correctness":0.25,"clarity":0.25,"engagement":0.25,"delivery":0.25}}`
	w, err = WeightsFromJSON(js)
	if err != nil {
		t.Fatalf("valid override: %v", err)
	}
	reel := w.For("reel")
	if !almostEqual(reel.Engagement, 0.4) {
		t.Errorf("reel engagement = %v, want 0.4", reel.Engagement)
	}
	// Built-in carousel still present (overlay, not replace-all).
	if w.For("carousel") != defaultProfiles["carousel"] {
		t.Errorf("override clobbered built-in carousel profile")
	}
	// Unconfigured slug now uses the overridden default (0.25 each).
	if !almostEqual(w.For("unknown").Correctness, 0.25) {
		t.Errorf("default override not applied: %+v", w.For("unknown"))
	}
}

func TestWeightsFromJSONErrors(t *testing.T) {
	// Malformed JSON.
	if _, err := WeightsFromJSON(`{not json`); err == nil {
		t.Error("expected error for malformed JSON")
	}
	// Profile that does not sum to 1.0 must be rejected.
	bad := `{"profiles":{"reel":{"correctness":0.5,"clarity":0.5,"engagement":0.5,"delivery":0.5}}}`
	if _, err := WeightsFromJSON(bad); err == nil {
		t.Error("expected error for profile not summing to 1.0")
	}
	// Profile that sums to 1.0 but has an out-of-range weight must be rejected
	// (1.5 + -0.5 + 0 + 0 = 1.0, so only the per-dimension check catches it).
	oor := `{"profiles":{"reel":{"correctness":1.5,"clarity":-0.5,"engagement":0,"delivery":0}}}`
	if _, err := WeightsFromJSON(oor); err == nil {
		t.Error("expected error for weight outside [0,1]")
	}
}

func TestProfileWeight(t *testing.T) {
	p := Profile{Correctness: 0.1, Clarity: 0.2, Engagement: 0.3, Delivery: 0.4}
	cases := map[models.EvaluationDimensionKey]float64{
		models.DimensionCorrectness:            0.1,
		models.DimensionClarity:                0.2,
		models.DimensionEngagement:             0.3,
		models.DimensionDelivery:               0.4,
		models.EvaluationDimensionKey("bogus"): 0,
	}
	for dim, want := range cases {
		if got := p.Weight(dim); !almostEqual(got, want) {
			t.Errorf("Weight(%q) = %v, want %v", dim, got, want)
		}
	}
}
