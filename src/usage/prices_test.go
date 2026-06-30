package usage_test

import (
	"testing"

	"github.com/ogen-app/ogen/src/usage"
	"github.com/ogen-app/ogen/src/vendors"
)

func TestApplyModelPrices(t *testing.T) {
	t.Run("empty is a no-op", func(t *testing.T) {
		if err := usage.ApplyModelPrices(""); err != nil {
			t.Fatalf("empty: %v", err)
		}
		if err := usage.ApplyModelPrices("   "); err != nil {
			t.Fatalf("blank: %v", err)
		}
	})

	t.Run("override merges + re-versions, keeps other models", func(t *testing.T) {
		js := `{"test-pricing":{"m":{"input":9000000,"output":15000000}}}`
		if err := usage.ApplyModelPrices(js); err != nil {
			t.Fatalf("apply: %v", err)
		}
		// 1M input @ $9/1M => 9_000_000 micros, tagged env-override.
		micros, ver, ok := vendors.CostOf("test-pricing", "m", vendors.Usage{vendors.KindInput: 1_000_000})
		if !ok || micros != 9_000_000 {
			t.Fatalf("CostOf(override) = (%d, %v), want (9000000, true)", micros, ok)
		}
		if ver != "env-override" {
			t.Errorf("version = %q, want env-override", ver)
		}
		// The pre-existing "keep" model survives the merge.
		if _, _, ok := vendors.CostOf("test-pricing", "keep", vendors.Usage{vendors.KindInput: 1}); !ok {
			t.Error("merge dropped the existing model 'keep'")
		}
	})

	t.Run("bad JSON errors", func(t *testing.T) {
		if err := usage.ApplyModelPrices(`{not json`); err == nil {
			t.Error("malformed JSON should error")
		}
	})

	t.Run("unknown vendor errors", func(t *testing.T) {
		if err := usage.ApplyModelPrices(`{"no-such-vendor":{"m":{"input":1}}}`); err == nil {
			t.Error("unknown vendor should error (fail fast at boot)")
		}
	})
}
