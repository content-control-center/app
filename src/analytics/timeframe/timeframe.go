// Package timeframe resolves the analytics dashboard window from request
// parameters and buckets timestamps within it. It is the single source of
// truth for "what window am I looking at" shared by the analytics endpoints
// (CON-237 overview, and CON-238/239 by reuse): from/to or a relative window
// shorthand, adaptive day/week granularity, the immediately-preceding
// equal-length window, and stable bucket boundaries for series alignment.
package timeframe

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Granularity is the bucket size of a series over a window.
type Granularity string

const (
	Day   Granularity = "day"
	Week  Granularity = "week"
	Month Granularity = "month"
)

// maxDays caps the resolvable span; a wider request is rejected rather than
// producing an unbounded series. 400 days keeps a full year of daily buckets in
// reach while staying comfortably bounded.
const maxDays = 400

var (
	// ErrInvalidRange is a malformed/reversed range or an unparseable window.
	ErrInvalidRange = errors.New("invalid_range")
	// ErrWindowTooLarge is a span beyond maxDays.
	ErrWindowTooLarge = errors.New("window_too_large")
)

// Range is a resolved window. From is inclusive and To is exclusive, both at
// UTC midnight; Days is the number of whole days in [From, To).
type Range struct {
	From        time.Time
	To          time.Time
	Days        int
	Granularity Granularity
}

var relativeRe = regexp.MustCompile(`^(\d+)(d|w|mo)$`)

// Resolve turns the query values into a Range. Either from+to (inclusive
// YYYY-MM-DD dates) or a relative window shorthand (e.g. "28d", "12w", "6mo";
// default "28d") may be given; from/to takes precedence. granStr optionally
// overrides the adaptive granularity. now is the reference "today" (UTC).
func Resolve(fromStr, toStr, windowStr, granStr string, now time.Time) (Range, error) {
	day := 24 * time.Hour
	today := truncateDay(now)

	var from, toExcl time.Time
	switch {
	case fromStr != "" || toStr != "":
		if fromStr == "" || toStr == "" {
			return Range{}, ErrInvalidRange
		}
		f, err := parseDate(fromStr)
		if err != nil {
			return Range{}, ErrInvalidRange
		}
		t, err := parseDate(toStr)
		if err != nil {
			return Range{}, ErrInvalidRange
		}
		if t.Before(f) {
			return Range{}, ErrInvalidRange
		}
		from, toExcl = f, t.Add(day) // to is inclusive
	default:
		days := 28
		if windowStr != "" {
			d, err := parseRelative(windowStr)
			if err != nil {
				return Range{}, ErrInvalidRange
			}
			days = d
		}
		toExcl = today.Add(day) // include today
		from = toExcl.Add(-time.Duration(days) * day)
	}

	days := int(toExcl.Sub(from) / day)
	if days < 1 {
		return Range{}, ErrInvalidRange
	}
	if days > maxDays {
		return Range{}, ErrWindowTooLarge
	}

	gran := adaptiveGranularity(days)
	if granStr != "" {
		g, ok := parseGranularity(granStr)
		if !ok {
			return Range{}, ErrInvalidRange
		}
		gran = g
	}
	return Range{From: from, To: toExcl, Days: days, Granularity: gran}, nil
}

// Previous returns the immediately-preceding equal-length window.
func (r Range) Previous() Range {
	span := r.To.Sub(r.From)
	return Range{From: r.From.Add(-span), To: r.From, Days: r.Days, Granularity: r.Granularity}
}

// FromISO / ToISO render the window's inclusive date bounds (To-1 day).
func (r Range) FromISO() string { return r.From.Format("2006-01-02") }
func (r Range) ToISO() string   { return r.To.Add(-24 * time.Hour).Format("2006-01-02") }

// Buckets returns the ordered bucket start times covering [From, To).
func (r Range) Buckets() []time.Time {
	var out []time.Time
	for t := r.bucketStart(r.From); t.Before(r.To); t = r.nextBucket(t) {
		out = append(out, t)
	}
	return out
}

// Labels returns the ISO date label for each bucket start.
func (r Range) Labels() []string {
	b := r.Buckets()
	out := make([]string, len(b))
	for i, t := range b {
		out[i] = t.Format("2006-01-02")
	}
	return out
}

// BucketIndex returns the index of the bucket containing t, or -1 if t falls
// outside [From, To).
func (r Range) BucketIndex(t time.Time) int {
	if t.Before(r.From) || !t.Before(r.To) {
		return -1
	}
	idx := 0
	for b := r.bucketStart(r.From); b.Before(r.To); b = r.nextBucket(b) {
		if !t.Before(b) && t.Before(r.nextBucket(b)) {
			return idx
		}
		idx++
	}
	return -1
}

// BucketEnd returns the exclusive end of the bucket that starts at t.
func (r Range) BucketEnd(t time.Time) time.Time { return r.nextBucket(t) }

func (r Range) bucketStart(t time.Time) time.Time {
	switch r.Granularity {
	case Week:
		// ISO week start (Monday).
		off := (int(t.Weekday()) + 6) % 7
		return truncateDay(t).AddDate(0, 0, -off)
	case Month:
		y, m, _ := t.Date()
		return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	default:
		return truncateDay(t)
	}
}

func (r Range) nextBucket(t time.Time) time.Time {
	switch r.Granularity {
	case Week:
		return t.AddDate(0, 0, 7)
	case Month:
		return t.AddDate(0, 1, 0)
	default:
		return t.AddDate(0, 0, 1)
	}
}

func truncateDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

func parseRelative(s string) (int, error) {
	m := relativeRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("bad window %q", s)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("bad window %q", s)
	}
	switch m[2] {
	case "d":
		return n, nil
	case "w":
		return n * 7, nil
	case "mo":
		return n * 30, nil
	}
	return 0, fmt.Errorf("bad window %q", s)
}

func parseGranularity(s string) (Granularity, bool) {
	switch Granularity(s) {
	case Day, Week, Month:
		return Granularity(s), true
	}
	return "", false
}

func adaptiveGranularity(days int) Granularity {
	switch {
	case days <= 90:
		return Day
	default:
		return Week
	}
}
