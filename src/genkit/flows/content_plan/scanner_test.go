package content_plan

import (
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
