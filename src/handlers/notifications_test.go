package handlers

import "testing"

func TestParseCursor(t *testing.T) {
	tests := []struct {
		name   string
		header string
		query  string
		want   int64
	}{
		{"empty", "", "", 0},
		{"header wins", "42", "7", 42},
		{"falls back to query", "", "7", 7},
		{"whitespace trimmed", "  15  ", "", 15},
		{"unparseable header ignored", "abc", "", 0},
		{"negative rejected", "-5", "", 0},
		{"header blank uses query", "  ", "9", 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseCursor(tc.header, tc.query); got != tc.want {
				t.Fatalf("parseCursor(%q, %q) = %d, want %d", tc.header, tc.query, got, tc.want)
			}
		})
	}
}
