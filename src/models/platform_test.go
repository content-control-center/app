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
