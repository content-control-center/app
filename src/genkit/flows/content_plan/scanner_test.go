package content_plan

import (
	"strings"
	"testing"
)

func TestJSONPostScanner(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
		want   []string
	}{
		{
			name:   "single object in one chunk",
			chunks: []string{`[{"title":"Hello"}]`},
			want:   []string{`{"title":"Hello"}`},
		},
		{
			name:   "multiple objects in one chunk",
			chunks: []string{`[{"a":"1"},{"b":"2"},{"c":"3"}]`},
			want:   []string{`{"a":"1"}`, `{"b":"2"}`, `{"c":"3"}`},
		},
		{
			name:   "object split across two chunks",
			chunks: []string{`[{"title":"He`, `llo"}]`},
			want:   []string{`{"title":"Hello"}`},
		},
		{
			name:   "object split across many chunks",
			chunks: []string{`[`, `{`, `"k"`, `:"v"`, `}`, `]`},
			want:   []string{`{"k":"v"}`},
		},
		{
			name:   "two objects split mid-boundary",
			chunks: []string{`[{"a":"1"},{`, `"b":"2"}]`},
			want:   []string{`{"a":"1"}`, `{"b":"2"}`},
		},
		{
			name:   "braces inside string values are ignored",
			chunks: []string{`[{"body":"use {braces} freely"}]`},
			want:   []string{`{"body":"use {braces} freely"}`},
		},
		{
			name:   "escaped quote inside string does not end string",
			chunks: []string{`[{"body":"say \"hi\""}]`},
			want:   []string{`{"body":"say \"hi\""}`},
		},
		{
			name:   "nested object inside post field",
			chunks: []string{`[{"meta":{"k":"v"},"title":"T"}]`},
			want:   []string{`{"meta":{"k":"v"},"title":"T"}`},
		},
		{
			name:   "empty array",
			chunks: []string{`[]`},
			want:   nil,
		},
		{
			name:   "whitespace between objects",
			chunks: []string{"[\n  {\"a\":\"1\"},\n  {\"b\":\"2\"}\n]"},
			want:   []string{`{"a":"1"}`, `{"b":"2"}`},
		},
		{
			name:   "markdown fence prefix is ignored gracefully",
			chunks: []string{"```json\n[{\"a\":\"1\"}]\n```"},
			want:   []string{`{"a":"1"}`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newJSONPostScanner()
			var got []string
			for _, chunk := range tt.chunks {
				got = append(got, s.push(chunk)...)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d objects, want %d\ngot:  %v\nwant: %v", len(got), len(tt.want), got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("object[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestTrimBody(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "short body unchanged",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "empty body unchanged",
			input: "",
			want:  "",
		},
		{
			name:  "exactly 500 runes unchanged",
			input: strings.Repeat("a", 500),
			want:  strings.Repeat("a", 500),
		},
		{
			name:  "501 runes truncated to 500",
			input: strings.Repeat("a", 501),
			want:  strings.Repeat("a", 500),
		},
		{
			name:  "multibyte unicode truncated by rune count not byte count",
			input: strings.Repeat("é", 501), // é is 2 bytes; must cut at 500 runes, not 500 bytes
			want:  strings.Repeat("é", 500),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimBody(tt.input)
			if got != tt.want {
				t.Errorf("trimBody: got %d runes, want %d", len([]rune(got)), len([]rune(tt.want)))
			}
		})
	}
}

func TestParseAndTrimPost(t *testing.T) {
	t.Run("valid JSON returns post", func(t *testing.T) {
		raw := `{"title":"Hello","body":"World","platformId":"abc","contentType":"text-post","publishDate":"2026-05-01","toneNotes":"","assetRefs":[]}`
		post, ok := parseAndTrimPost(raw)
		if !ok {
			t.Fatal("expected ok=true, got false")
		}
		if post.Title != "Hello" {
			t.Errorf("title: got %q, want %q", post.Title, "Hello")
		}
		if post.Body != "World" {
			t.Errorf("body: got %q, want %q", post.Body, "World")
		}
	})

	t.Run("body over 500 runes is truncated", func(t *testing.T) {
		longBody := strings.Repeat("x", 600)
		raw := `{"title":"T","body":"` + longBody + `"}`
		post, ok := parseAndTrimPost(raw)
		if !ok {
			t.Fatal("expected ok=true, got false")
		}
		if got := len([]rune(post.Body)); got != 500 {
			t.Errorf("body rune count: got %d, want 500", got)
		}
	})

	t.Run("malformed JSON returns false", func(t *testing.T) {
		_, ok := parseAndTrimPost(`{"title": bad json`)
		if ok {
			t.Error("expected ok=false for malformed JSON, got true")
		}
	})

	t.Run("empty string returns false", func(t *testing.T) {
		_, ok := parseAndTrimPost("")
		if ok {
			t.Error("expected ok=false for empty input, got true")
		}
	})
}

func TestTailOf(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{"short string returned as-is", "hello", 10, "hello"},
		{"exact length returned as-is", "hello", 5, "hello"},
		{"longer string gets prefix", "hello world", 5, "...world"},
		{"empty string", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tailOf(tt.s, tt.n)
			if got != tt.want {
				t.Errorf("tailOf(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}
