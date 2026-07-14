package jsonstream

import (
	"strings"
	"testing"
)

type capturedDelta struct {
	key   string
	delta string
}

func collect(watched ...string) (*Scanner, *[]capturedDelta) {
	var out []capturedDelta
	s := New(watched, func(k, d string) {
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

// ── Values() tests ──────────────────────────────────────────────────────────
//
// These cover the authoritative extraction path: feed raw model output,
// call Values(), and confirm the typed map has the right shape. The
// scanner bypasses encoding/json so it tolerates drift patterns that
// would break json.Unmarshal.

// feed pushes the whole input in one call and returns the Values map.
func feed(md string) map[string]any {
	s := New(nil, nil)
	s.Push(md)
	return s.Values()
}

func TestValues_AllStringFields(t *testing.T) {
	v := feed(`{"explanation":"hi","updatedContent":"body","action":"edited","saveVersion":false,"versionNote":""}`)
	if v["explanation"] != "hi" {
		t.Errorf("explanation: got %v", v["explanation"])
	}
	if v["updatedContent"] != "body" {
		t.Errorf("updatedContent: got %v", v["updatedContent"])
	}
	if v["action"] != "edited" {
		t.Errorf("action: got %v", v["action"])
	}
	if v["saveVersion"] != false {
		t.Errorf("saveVersion: got %v (%T), want false", v["saveVersion"], v["saveVersion"])
	}
	if v["versionNote"] != "" {
		t.Errorf("versionNote: got %v", v["versionNote"])
	}
}

func TestValues_BoolAndNullAndNumber(t *testing.T) {
	v := feed(`{"a":true,"b":false,"c":null,"d":42,"e":3.14}`)
	if v["a"] != true {
		t.Errorf("a: got %v (%T)", v["a"], v["a"])
	}
	if v["b"] != false {
		t.Errorf("b: got %v (%T)", v["b"], v["b"])
	}
	if v["c"] != nil {
		t.Errorf("c: got %v, want nil", v["c"])
	}
	if v["d"] != 42.0 {
		t.Errorf("d: got %v (%T), want 42.0", v["d"], v["d"])
	}
	if v["e"] != 3.14 {
		t.Errorf("e: got %v (%T), want 3.14", v["e"], v["e"])
	}
}

func TestValues_TrailingCommasTolerated(t *testing.T) {
	v := feed(`{"a":"b","c":"d",}`)
	if v["a"] != "b" || v["c"] != "d" {
		t.Errorf("trailing comma not tolerated: %#v", v)
	}
}

func TestValues_MissingCommasTolerated(t *testing.T) {
	// Claude occasionally drops the separator — the scanner's state
	// machine treats the next `"` at top level as the start of a new key.
	v := feed(`{"a":"b" "c":"d"}`)
	if v["a"] != "b" || v["c"] != "d" {
		t.Errorf("missing comma not tolerated: %#v", v)
	}
}

func TestValues_MissingCommaAfterBool(t *testing.T) {
	// Literal collection ends at comma or `}`; we rely on state to
	// recognise the next `"` as a new key opening. This catches what the
	// regex-based repair pipeline missed.
	v := feed(`{"saveVersion":true "versionNote":"ok"}`)
	if v["saveVersion"] != true {
		t.Errorf("saveVersion: got %v (%T)", v["saveVersion"], v["saveVersion"])
	}
	if v["versionNote"] != "ok" {
		t.Errorf("versionNote: got %v", v["versionNote"])
	}
}

func TestValues_PreambleAndPostamble(t *testing.T) {
	v := feed(`Sure, here's the response: {"a":"b","c":"d"} Let me know!`)
	if v["a"] != "b" || v["c"] != "d" {
		t.Errorf("prose not ignored: %#v", v)
	}
}

func TestValues_MarkdownCodeFence(t *testing.T) {
	v := feed("```json\n{\"a\":\"b\"}\n```")
	if v["a"] != "b" {
		t.Errorf("fence not ignored: %#v", v)
	}
}

func TestValues_LiteralNewlineInStringTolerated(t *testing.T) {
	// Strict JSON requires \n escape for newlines inside strings; Claude
	// occasionally emits raw LF. The scanner treats any non-\ non-" byte
	// as content, so a raw newline is accumulated into the value.
	v := feed("{\"explanation\":\"line1\nline2\"}")
	if got, ok := v["explanation"].(string); !ok || got != "line1\nline2" {
		t.Errorf("explanation: got %q (%T)", v["explanation"], v["explanation"])
	}
}

func TestValues_TruncatedString(t *testing.T) {
	// Stream cut off mid-string (max_tokens hit). The partial value is
	// still available; downstream code can decide whether to surface it.
	v := feed(`{"explanation":"hello wor`)
	if got, ok := v["explanation"].(string); !ok || got != "hello wor" {
		t.Errorf("explanation: got %q (%T)", v["explanation"], v["explanation"])
	}
}

func TestValues_TruncatedLiteral(t *testing.T) {
	// Cut off inside a literal "true" — we return the raw partial
	// because float parse fails. Caller can treat as missing.
	v := feed(`{"explanation":"hi","saveVersion":tru`)
	if v["explanation"] != "hi" {
		t.Errorf("explanation lost during truncation: got %v", v["explanation"])
	}
	if got, ok := v["saveVersion"].(string); !ok || got != "tru" {
		t.Errorf("saveVersion truncation: got %v (%T)", v["saveVersion"], v["saveVersion"])
	}
}

func TestValues_MissingField(t *testing.T) {
	v := feed(`{"explanation":"e","action":"edited"}`)
	if _, present := v["saveVersion"]; present {
		t.Errorf("missing saveVersion should be absent, got %v", v["saveVersion"])
	}
	if _, present := v["updatedContent"]; present {
		t.Errorf("missing updatedContent should be absent")
	}
}

func TestValues_DuplicateKeyLastWins(t *testing.T) {
	v := feed(`{"a":"first","a":"second"}`)
	if v["a"] != "second" {
		t.Errorf("duplicate key: got %v, want second", v["a"])
	}
}

func TestValues_NestedObjectSkipped(t *testing.T) {
	// Nested objects are not extracted — we don't need them. The scanner
	// steps past them cleanly so following top-level keys still parse.
	v := feed(`{"meta":{"k":"v"},"explanation":"after"}`)
	if v["explanation"] != "after" {
		t.Errorf("explanation after nested object: got %v", v["explanation"])
	}
	if _, present := v["meta"]; present {
		t.Errorf("nested object should not appear in Values")
	}
}

func TestValues_EscapesDecoded(t *testing.T) {
	v := feed(`{"explanation":"line\nhere\t\"quoted\" \\end"}`)
	want := "line\nhere\t\"quoted\" \\end"
	if got := v["explanation"]; got != want {
		t.Errorf("escapes: got %q, want %q", got, want)
	}
}

func TestValues_WithWatchedKeyDeltasAndValues(t *testing.T) {
	// The watched-key path and the accumulator path are independent:
	// deltas fire as bytes arrive, Values() returns the full accumulated
	// value when the stream ends.
	s, caps := collect("explanation")
	for _, c := range []string{`{"expl`, `anation":"he`, `llo","action":"edited"}`} {
		s.Push(c)
	}
	if joined(*caps, "explanation") != "hello" {
		t.Errorf("deltas: got %q", joined(*caps, "explanation"))
	}
	v := s.Values()
	if v["explanation"] != "hello" {
		t.Errorf("Values explanation: got %v", v["explanation"])
	}
	if v["action"] != "edited" {
		t.Errorf("Values action: got %v", v["action"])
	}
}

func TestValues_ChunkBoundariesInsideLiteral(t *testing.T) {
	s := New(nil, nil)
	s.Push(`{"saveVersion":tr`)
	s.Push(`ue,"action":"edited"}`)
	v := s.Values()
	if v["saveVersion"] != true {
		t.Errorf("saveVersion: got %v (%T)", v["saveVersion"], v["saveVersion"])
	}
	if v["action"] != "edited" {
		t.Errorf("action: got %v", v["action"])
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
