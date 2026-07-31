package models

import "testing"

func TestTextConstraints_ValueScanRoundTrip(t *testing.T) {
	in := TextConstraints{
		MaxContentChars: 3000,
		MaxTitleChars:   100,
		PerPostType:     map[string]int{"article": 100000},
	}
	v, err := in.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	var out TextConstraints
	if err := out.Scan(v); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if out.MaxContentChars != 3000 || out.MaxTitleChars != 100 || out.PerPostType["article"] != 100000 {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestTextConstraints_ScanEmptyIsZero(t *testing.T) {
	var c TextConstraints
	for _, src := range []any{"", []byte(nil), "{}", nil} {
		if err := c.Scan(src); err != nil {
			t.Fatalf("Scan(%v): %v", src, err)
		}
		if !c.IsZero() {
			t.Fatalf("Scan(%v): want zero value, got %+v", src, c)
		}
	}
}

func TestTextConstraints_ScanReplacesPriorState(t *testing.T) {
	// A fully populated value (map override + title cap) followed by a leaner
	// one that omits both must not leak the prior state — regression for a
	// receiver reused across scans (json.Unmarshal merges into maps).
	full := `{"max_content_chars":3000,"max_title_chars":100,"per_post_type":{"article":100000}}`
	lean := `{"max_content_chars":500}`

	for _, tc := range []struct {
		name         string
		first, later any
	}{
		{"string", full, lean},
		{"bytes", []byte(full), []byte(lean)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var c TextConstraints
			if err := c.Scan(tc.first); err != nil {
				t.Fatalf("first Scan: %v", err)
			}
			if err := c.Scan(tc.later); err != nil {
				t.Fatalf("second Scan: %v", err)
			}
			if c.MaxContentChars != 500 {
				t.Errorf("max_content_chars: want 500, got %d", c.MaxContentChars)
			}
			if c.MaxTitleChars != 0 {
				t.Errorf("max_title_chars: prior value leaked, want 0, got %d", c.MaxTitleChars)
			}
			if c.PerPostType != nil {
				t.Errorf("per_post_type: prior map leaked, want nil, got %+v", c.PerPostType)
			}
		})
	}
}

func TestTextConstraints_ContentLimitFor(t *testing.T) {
	c := TextConstraints{MaxContentChars: 280, PerPostType: map[string]int{"long-form-post": 25000}}
	if got := c.ContentLimitFor("text-post"); got != 280 {
		t.Errorf("text-post: want default 280, got %d", got)
	}
	if got := c.ContentLimitFor("long-form-post"); got != 25000 {
		t.Errorf("long-form-post: want override 25000, got %d", got)
	}

	// A platform with no default and no override yields 0 (unbounded).
	if got := (TextConstraints{}).ContentLimitFor("text-post"); got != 0 {
		t.Errorf("empty: want 0, got %d", got)
	}
}
