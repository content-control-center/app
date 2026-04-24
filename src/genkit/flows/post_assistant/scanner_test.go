package post_assistant

import (
	"strings"
	"testing"
)

type capturedDelta struct {
	key   string
	delta string
}

func collect(watched ...string) (*JSONStringScanner, *[]capturedDelta) {
	var out []capturedDelta
	s := NewJSONStringScanner(watched, func(k, d string) {
		out = append(out, capturedDelta{k, d})
	})
	return s, &out
}

func joined(caps []capturedDelta, key string) string {
	var b strings.Builder
	for _, c := range caps {
		if c.key == key {
			b.WriteString(c.delta)
		}
	}
	return b.String()
}

func TestScanner_SingleStringInOneChunk(t *testing.T) {
	s, caps := collect("explanation")
	s.Push(`{"explanation":"hello"}`)
	if got := joined(*caps, "explanation"); got != "hello" {
		t.Fatalf("explanation: got %q, want %q", got, "hello")
	}
}

func TestScanner_SplitAcrossManyChunks(t *testing.T) {
	s, caps := collect("explanation")
	for _, c := range []string{`{`, `"expl`, `anation":"`, `hel`, `lo world`, `"}`} {
		s.Push(c)
	}
	if got := joined(*caps, "explanation"); got != "hello world" {
		t.Fatalf("explanation: got %q, want %q", got, "hello world")
	}
}

func TestScanner_EscapedCharsDecoded(t *testing.T) {
	s, caps := collect("explanation")
	s.Push(`{"explanation":"line1\nline2\ttabbed \"quoted\" \\slash\/end"}`)
	want := "line1\nline2\ttabbed \"quoted\" \\slash/end"
	if got := joined(*caps, "explanation"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScanner_EscapeSplitAcrossChunks(t *testing.T) {
	// backslash ends one chunk, the escape target starts the next
	s, caps := collect("explanation")
	s.Push(`{"explanation":"a\`)
	s.Push(`nb"}`)
	want := "a\nb"
	if got := joined(*caps, "explanation"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScanner_UnicodeEscape(t *testing.T) {
	s, caps := collect("explanation")
	s.Push(`{"explanation":"snowman ☃!"}`)
	want := "snowman ☃!"
	if got := joined(*caps, "explanation"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScanner_UnicodeEscapeSplitAcrossChunks(t *testing.T) {
	// chunk boundary falls inside the 4 hex digits
	s, caps := collect("explanation")
	s.Push(`{"explanation":"x\u26`)
	s.Push(`03y"}`)
	want := "x☃y"
	if got := joined(*caps, "explanation"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScanner_SurrogatePair(t *testing.T) {
	// U+1F600 (😀) as a surrogate pair 😀
	s, caps := collect("explanation")
	s.Push(`{"explanation":"smile 😀!"}`)
	want := "smile \U0001F600!"
	if got := joined(*caps, "explanation"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScanner_MultibyteUTF8SplitAcrossChunks(t *testing.T) {
	// é is C3 A9; split between the two bytes.
	s, caps := collect("explanation")
	s.Push("{\"explanation\":\"caf\xc3")
	s.Push("\xa9 done\"}")
	want := "café done"
	if got := joined(*caps, "explanation"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestScanner_WatchedSecondField(t *testing.T) {
	s, caps := collect("updatedContent")
	s.Push(`{"explanation":"ignored","updatedContent":"# Title\n\nBody"}`)
	if got := joined(*caps, "explanation"); got != "" {
		t.Fatalf("explanation should be empty, got %q", got)
	}
	want := "# Title\n\nBody"
	if got := joined(*caps, "updatedContent"); got != want {
		t.Fatalf("updatedContent: got %q, want %q", got, want)
	}
}

func TestScanner_BothWatchedKeysFired(t *testing.T) {
	s, caps := collect("explanation", "updatedContent")
	s.Push(`{"explanation":"hi","updatedContent":"body","action":"edited","saveVersion":false}`)
	if got := joined(*caps, "explanation"); got != "hi" {
		t.Fatalf("explanation: got %q", got)
	}
	if got := joined(*caps, "updatedContent"); got != "body" {
		t.Fatalf("updatedContent: got %q", got)
	}
}

func TestScanner_NonStringValuesSkipped(t *testing.T) {
	s, caps := collect("explanation", "updatedContent")
	s.Push(`{"saveVersion":true,"count":42,"empty":null,"explanation":"e"}`)
	if got := joined(*caps, "explanation"); got != "e" {
		t.Fatalf("explanation: got %q, want %q", got, "e")
	}
}

func TestScanner_NestedObjectSkipped(t *testing.T) {
	s, caps := collect("explanation")
	s.Push(`{"meta":{"k":"v","nested":{"x":1}},"explanation":"after"}`)
	if got := joined(*caps, "explanation"); got != "after" {
		t.Fatalf("explanation: got %q", got)
	}
}

func TestScanner_NestedArraySkipped(t *testing.T) {
	s, caps := collect("explanation")
	s.Push(`{"tags":["a","b"],"explanation":"hi"}`)
	if got := joined(*caps, "explanation"); got != "hi" {
		t.Fatalf("explanation: got %q", got)
	}
}

func TestScanner_BracesInsideStringIgnored(t *testing.T) {
	s, caps := collect("explanation")
	s.Push(`{"explanation":"use {braces} freely"}`)
	if got := joined(*caps, "explanation"); got != "use {braces} freely" {
		t.Fatalf("got %q", got)
	}
}

func TestScanner_MarkdownFencePreamble(t *testing.T) {
	s, caps := collect("explanation")
	s.Push("```json\n{\"explanation\":\"fenced\"}\n```")
	if got := joined(*caps, "explanation"); got != "fenced" {
		t.Fatalf("got %q", got)
	}
}

func TestScanner_WhitespaceInsideObject(t *testing.T) {
	s, caps := collect("explanation")
	s.Push(`{  "explanation"  :  "hi"  }`)
	if got := joined(*caps, "explanation"); got != "hi" {
		t.Fatalf("got %q", got)
	}
}

func TestScanner_FullTextPreserved(t *testing.T) {
	s, _ := collect("explanation")
	chunks := []string{`{"expl`, `anation":"hi","action":"edited"}`}
	for _, c := range chunks {
		s.Push(c)
	}
	want := `{"explanation":"hi","action":"edited"}`
	if got := s.FullText(); got != want {
		t.Fatalf("FullText: got %q, want %q", got, want)
	}
}

func TestTrimIncompleteUTF8(t *testing.T) {
	tests := []struct {
		name string
		in   string
		full string
		rest string
	}{
		{"ascii only", "hello", "hello", ""},
		{"complete 2-byte", "caf\xc3\xa9", "caf\xc3\xa9", ""},
		{"partial 2-byte", "caf\xc3", "caf", "\xc3"},
		{"partial 3-byte one byte", "x\xe2", "x", "\xe2"},
		{"partial 3-byte two bytes", "x\xe2\x98", "x", "\xe2\x98"},
		{"complete 3-byte", "x\xe2\x98\x83", "x\xe2\x98\x83", ""},
		{"partial 4-byte three bytes", "\xf0\x9f\x98", "", "\xf0\x9f\x98"},
		{"complete 4-byte", "\xf0\x9f\x98\x80", "\xf0\x9f\x98\x80", ""},
		{"empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			full, rest := trimIncompleteUTF8([]byte(tt.in))
			if string(full) != tt.full || string(rest) != tt.rest {
				t.Fatalf("got (%q,%q), want (%q,%q)", full, rest, tt.full, tt.rest)
			}
		})
	}
}
