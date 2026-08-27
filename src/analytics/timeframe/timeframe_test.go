package timeframe

import (
	"errors"
	"testing"
	"time"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestResolveRelativeDefault(t *testing.T) {
	now := date("2026-08-27").Add(13 * time.Hour) // mid-day, must be truncated
	r, err := Resolve("", "", "", "", now)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if r.Days != 28 {
		t.Fatalf("days = %d, want 28", r.Days)
	}
	if r.Granularity != Day {
		t.Fatalf("granularity = %s, want day", r.Granularity)
	}
	// To is exclusive = start of tomorrow; inclusive ToISO = today.
	if got := r.ToISO(); got != "2026-08-27" {
		t.Fatalf("toISO = %s, want 2026-08-27", got)
	}
	if got := r.FromISO(); got != "2026-07-31" {
		t.Fatalf("fromISO = %s, want 2026-07-31", got)
	}
	if got := len(r.Buckets()); got != 28 {
		t.Fatalf("buckets = %d, want 28", got)
	}
}

func TestResolveExplicitRange(t *testing.T) {
	now := date("2026-08-27")
	r, err := Resolve("2026-08-01", "2026-08-07", "", "", now)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if r.Days != 7 {
		t.Fatalf("days = %d, want 7", r.Days)
	}
	if len(r.Buckets()) != 7 {
		t.Fatalf("buckets = %d, want 7", len(r.Buckets()))
	}
	if r.ToISO() != "2026-08-07" || r.FromISO() != "2026-08-01" {
		t.Fatalf("range = %s..%s", r.FromISO(), r.ToISO())
	}
}

func TestResolveErrors(t *testing.T) {
	now := date("2026-08-27")
	cases := []struct {
		name             string
		from, to, window string
		want             error
	}{
		{"reversed", "2026-08-07", "2026-08-01", "", ErrInvalidRange},
		{"one-sided", "2026-08-07", "", "", ErrInvalidRange},
		{"bad-window", "", "", "banana", ErrInvalidRange},
		{"too-large", "", "", "500d", ErrWindowTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(tc.from, tc.to, tc.window, "", now)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestPreviousWindow(t *testing.T) {
	now := date("2026-08-27")
	r, _ := Resolve("2026-08-01", "2026-08-07", "", "", now)
	prev := r.Previous()
	if prev.ToISO() != "2026-07-31" || prev.FromISO() != "2026-07-25" {
		t.Fatalf("prev = %s..%s, want 2026-07-25..2026-07-31", prev.FromISO(), prev.ToISO())
	}
	if prev.Days != r.Days {
		t.Fatalf("prev days = %d, want %d", prev.Days, r.Days)
	}
}

func TestBucketIndex(t *testing.T) {
	now := date("2026-08-27")
	r, _ := Resolve("2026-08-01", "2026-08-07", "", "", now)
	if got := r.BucketIndex(date("2026-08-01").Add(6 * time.Hour)); got != 0 {
		t.Fatalf("index day0 = %d, want 0", got)
	}
	if got := r.BucketIndex(date("2026-08-07").Add(6 * time.Hour)); got != 6 {
		t.Fatalf("index day6 = %d, want 6", got)
	}
	if got := r.BucketIndex(date("2026-09-01")); got != -1 {
		t.Fatalf("index out-of-range = %d, want -1", got)
	}
}

func TestWeekGranularity(t *testing.T) {
	now := date("2026-08-27")
	// 120 days > 90 → weekly buckets.
	r, err := Resolve("", "", "120d", "", now)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if r.Granularity != Week {
		t.Fatalf("granularity = %s, want week", r.Granularity)
	}
	// weekly buckets over ~120 days is ~18.
	if n := len(r.Buckets()); n < 17 || n > 19 {
		t.Fatalf("weekly buckets = %d, want ~18", n)
	}
}
