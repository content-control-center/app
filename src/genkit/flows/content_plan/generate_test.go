package content_plan

import "testing"

// CON-114: the streaming persist path stops at the batch's requested count, so
// an over-producing model can't turn "generate exactly 1" into 3 persisted
// posts. expectedCount <= 0 is the uncapped fallback (model decides the count).
func TestWithinCount(t *testing.T) {
	cases := []struct {
		name                string
		persisted, expected int
		want                bool
	}{
		{"first post for a 1-post batch", 0, 1, true},
		{"second post exceeds a 1-post batch", 1, 1, false},
		{"under a 3-post batch", 2, 3, true},
		{"at a 3-post batch cap", 3, 3, false},
		{"uncapped (zero)", 5, 0, true},
		{"uncapped (negative)", 5, -1, true},
	}
	for _, c := range cases {
		if got := withinCount(c.persisted, c.expected); got != c.want {
			t.Errorf("%s: withinCount(%d, %d) = %v, want %v", c.name, c.persisted, c.expected, got, c.want)
		}
	}
}
