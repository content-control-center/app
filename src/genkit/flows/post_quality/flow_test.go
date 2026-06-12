package post_quality

import "testing"

// inputHash is the change-detection fingerprint (CON-92): the assess flow
// re-runs the model only when this value differs from the stored one. These
// tests pin the two properties the cache correctness depends on — it is
// deterministic for identical inputs, and it changes whenever any field the
// model actually sees (system prompt, user prompt, or model id) changes.
func TestInputHash(t *testing.T) {
	base := &renderedPrompts{system: "sys", user: "usr"}
	want := inputHash(base, "claude-sonnet")

	t.Run("deterministic for identical inputs", func(t *testing.T) {
		got := inputHash(&renderedPrompts{system: "sys", user: "usr"}, "claude-sonnet")
		if got != want {
			t.Fatalf("identical inputs produced different hashes:\n %s\n %s", want, got)
		}
	})

	t.Run("captionScoped does not affect the hash", func(t *testing.T) {
		// captionScoped is a derived flag, not part of what the model reads,
		// so it must not perturb the fingerprint.
		got := inputHash(&renderedPrompts{system: "sys", user: "usr", captionScoped: true}, "claude-sonnet")
		if got != want {
			t.Fatalf("captionScoped changed the hash; it should not")
		}
	})

	t.Run("changes when any model-visible field changes", func(t *testing.T) {
		cases := map[string]string{
			"system": inputHash(&renderedPrompts{system: "SYS", user: "usr"}, "claude-sonnet"),
			"user":   inputHash(&renderedPrompts{system: "sys", user: "USR"}, "claude-sonnet"),
			"model":  inputHash(base, "claude-haiku"),
		}
		for field, got := range cases {
			if got == want {
				t.Errorf("changing %s did not change the hash", field)
			}
		}
	})

	t.Run("separator prevents field bleed", func(t *testing.T) {
		// Without a delimiter between fields, ("ab","c") and ("a","bc")
		// would hash identically. The unit-separator must keep them distinct.
		a := inputHash(&renderedPrompts{system: "ab", user: "c"}, "m")
		b := inputHash(&renderedPrompts{system: "a", user: "bc"}, "m")
		if a == b {
			t.Fatalf("adjacent fields collided: separator not effective")
		}
	})
}
