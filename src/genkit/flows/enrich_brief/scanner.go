package enrich_brief

import (
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// JSONStringScanner is an incremental parser for a single top-level JSON
// object. It serves two purposes:
//
//   - During a stream, it invokes onDelta with decoded fragments of watched
//     top-level string keys so the UI can preview text as it arrives.
//
//   - After the stream finishes, Values() returns a map of every top-level
//     key's parsed value — strings decoded, literals (true/false/null and
//     numbers) coerced to their Go equivalents. This is the authoritative
//     extraction path; it bypasses encoding/json entirely and therefore
//     tolerates trailing commas, missing separators, preamble/postamble
//     prose, literal newlines inside strings, and truncation. Only keys
//     whose values the scanner actually observed are present in the map.
//
// The scanner is forgiving of preamble (markdown fences, prose) before the
// opening `{` and of trailing text after the closing `}`.
type JSONStringScanner struct {
	watched map[string]bool
	onDelta func(key, delta string)

	state    scannerState
	esc      escState
	keyBuf   []byte
	hexBuf   []byte
	pendHigh rune // high surrogate awaiting its low pair
	curKey   string
	watching bool

	nestedDepth int

	// deltaBuf accumulates decoded bytes of the currently-streaming watched
	// string value. It is flushed at the end of every Push call.
	deltaBuf []byte
	// carry holds trailing bytes of an incomplete UTF-8 sequence that must
	// be prepended to deltaBuf before the next flush.
	carry []byte

	// fullText accumulates the raw input bytes — useful for debugging but
	// not used by the authoritative extraction (which goes via accumulators).
	fullText []byte

	// accumulators stores per-key complete values. Populated as values
	// stream in (strings decoded, literals captured raw); snapshotted by
	// Values() into a typed map[string]any.
	accumulators map[string]*valueAcc
}

type valueKind int

const (
	valKindString valueKind = iota
	valKindLiteral
)

type valueAcc struct {
	kind valueKind
	buf  []byte
}

type scannerState int

const (
	stPreamble       scannerState = iota
	stTop                          // between keys at the top level
	stInKey
	stAfterKey
	stAwaitValue
	stInString
	stCollectLiteral // inside a non-string value (bool / number / null) — accumulate bytes
	stAfterValue     // value just closed, looking for "," or "}"
	stInNested
	stDone
)

type escState int

const (
	escNone escState = iota
	escBackslash
	escUnicode
)

// NewJSONStringScanner returns a scanner that calls onDelta whenever new
// decoded bytes of a watched string key are available. watched may be nil
// if callers only need Values() (no streaming preview).
func NewJSONStringScanner(watched []string, onDelta func(key, delta string)) *JSONStringScanner {
	m := make(map[string]bool, len(watched))
	for _, k := range watched {
		m[k] = true
	}
	return &JSONStringScanner{
		watched:      m,
		onDelta:      onDelta,
		accumulators: make(map[string]*valueAcc),
	}
}

// Push feeds a chunk of raw text from the model stream. It appends the
// chunk to the internal raw buffer, advances the state machine, and flushes
// any pending decoded delta at the end.
func (s *JSONStringScanner) Push(chunk string) {
	s.fullText = append(s.fullText, chunk...)
	for i := 0; i < len(chunk); i++ {
		s.step(chunk[i])
	}
	s.flushDelta()
}

// FullText returns every byte pushed into the scanner so far.
func (s *JSONStringScanner) FullText() string {
	return string(s.fullText)
}

// Values snapshots the scanner's per-key accumulators into a typed map.
// Strings are decoded; "true"/"false"/"null" become bool/nil; other
// literals are parsed as float64 or returned as the raw trimmed string
// on parse failure. Keys that the scanner did not see (including those
// whose value was a nested object/array, which is skipped) are absent.
func (s *JSONStringScanner) Values() map[string]any {
	out := make(map[string]any, len(s.accumulators))
	for key, acc := range s.accumulators {
		if acc == nil {
			continue
		}
		switch acc.kind {
		case valKindString:
			out[key] = string(acc.buf)
		case valKindLiteral:
			raw := strings.TrimSpace(string(acc.buf))
			switch raw {
			case "true":
				out[key] = true
			case "false":
				out[key] = false
			case "null":
				out[key] = nil
			default:
				if n, err := strconv.ParseFloat(raw, 64); err == nil {
					out[key] = n
				} else {
					out[key] = raw
				}
			}
		}
	}
	return out
}

func (s *JSONStringScanner) step(c byte) {
	switch s.state {
	case stPreamble:
		if c == '{' {
			s.state = stTop
		}
	case stTop:
		switch c {
		case '"':
			s.keyBuf = s.keyBuf[:0]
			s.esc = escNone
			s.state = stInKey
		case '}':
			s.state = stDone
		}
	case stInKey:
		// Keys are kept simple — no \uXXXX support needed in practice.
		if s.esc == escBackslash {
			s.keyBuf = append(s.keyBuf, c)
			s.esc = escNone
			return
		}
		switch c {
		case '\\':
			s.esc = escBackslash
		case '"':
			s.curKey = string(s.keyBuf)
			s.watching = s.watched[s.curKey]
			s.state = stAfterKey
		default:
			s.keyBuf = append(s.keyBuf, c)
		}
	case stAfterKey:
		if c == ':' {
			s.state = stAwaitValue
		}
	case stAwaitValue:
		switch c {
		case ' ', '\t', '\n', '\r':
			// skip whitespace
		case '"':
			s.esc = escNone
			s.hexBuf = s.hexBuf[:0]
			s.pendHigh = 0
			s.resetAccumulator(s.curKey, valKindString)
			s.state = stInString
		case '{', '[':
			// Nested structure — we don't extract these, but still skip
			// cleanly so following keys are parsed.
			s.nestedDepth = 1
			s.state = stInNested
		default:
			// bool / number / null literal — accumulate from the first byte.
			s.resetAccumulator(s.curKey, valKindLiteral)
			s.appendToAccumulator(c)
			s.state = stCollectLiteral
		}
	case stInString:
		s.stringByte(c)
	case stCollectLiteral:
		switch c {
		case ',':
			s.resetKey()
			s.state = stTop
		case '}':
			s.state = stDone
		case '"':
			// Missing comma recovery: a quote mid-literal-tail is
			// almost certainly the start of the next key. Terminate the
			// literal and jump into key-parsing.
			s.resetKey()
			s.keyBuf = s.keyBuf[:0]
			s.esc = escNone
			s.state = stInKey
		case ' ', '\t', '\n', '\r':
			// Whitespace ends the literal; wait for the separator
			// (or a missing-comma quote) in stAfterValue.
			s.state = stAfterValue
		default:
			s.appendToAccumulator(c)
		}
	case stAfterValue:
		switch c {
		case ',':
			s.resetKey()
			s.state = stTop
		case '}':
			s.state = stDone
		case '"':
			// Missing comma recovery: next key starts here.
			s.resetKey()
			s.keyBuf = s.keyBuf[:0]
			s.esc = escNone
			s.state = stInKey
		}
	case stInNested:
		switch c {
		case '{', '[':
			s.nestedDepth++
		case '}', ']':
			s.nestedDepth--
			if s.nestedDepth == 0 {
				s.resetKey()
				s.state = stTop
			}
		}
	case stDone:
		// ignore everything after top-level close
	}
}

func (s *JSONStringScanner) resetKey() {
	s.curKey = ""
	s.watching = false
}

// resetAccumulator initialises or overwrites the accumulator for key.
// Overwrite semantics give "last-wins" behaviour for duplicate keys.
func (s *JSONStringScanner) resetAccumulator(key string, kind valueKind) {
	if key == "" {
		return
	}
	s.accumulators[key] = &valueAcc{kind: kind}
}

// appendToAccumulator writes a raw byte to the current key's accumulator.
// Used by the literal-collection path.
func (s *JSONStringScanner) appendToAccumulator(c byte) {
	if s.curKey == "" {
		return
	}
	if acc := s.accumulators[s.curKey]; acc != nil {
		acc.buf = append(acc.buf, c)
	}
}

func (s *JSONStringScanner) stringByte(c byte) {
	switch s.esc {
	case escNone:
		switch c {
		case '\\':
			s.esc = escBackslash
		case '"':
			// End of string value. Transition state first so flushDelta
			// treats this as "no more bytes coming for this key" and emits
			// any dangling partial-UTF-8 carry instead of retaining it.
			s.state = stAfterValue
			s.flushDelta()
			s.resetKey()
			s.carry = s.carry[:0]
		default:
			s.appendByte(c)
		}
	case escBackslash:
		switch c {
		case '"':
			s.appendByte('"')
			s.esc = escNone
		case '\\':
			s.appendByte('\\')
			s.esc = escNone
		case '/':
			s.appendByte('/')
			s.esc = escNone
		case 'n':
			s.appendByte('\n')
			s.esc = escNone
		case 't':
			s.appendByte('\t')
			s.esc = escNone
		case 'r':
			s.appendByte('\r')
			s.esc = escNone
		case 'b':
			s.appendByte('\b')
			s.esc = escNone
		case 'f':
			s.appendByte('\f')
			s.esc = escNone
		case 'u':
			s.hexBuf = s.hexBuf[:0]
			s.esc = escUnicode
		default:
			// malformed — emit the byte verbatim so nothing is silently lost
			s.appendByte(c)
			s.esc = escNone
		}
	case escUnicode:
		s.hexBuf = append(s.hexBuf, c)
		if len(s.hexBuf) == 4 {
			v, err := strconv.ParseUint(string(s.hexBuf), 16, 32)
			if err == nil {
				r := rune(v)
				switch {
				case utf16.IsSurrogate(r) && r >= 0xD800 && r <= 0xDBFF:
					// high surrogate — wait for the low half
					s.pendHigh = r
				case utf16.IsSurrogate(r) && r >= 0xDC00 && r <= 0xDFFF:
					if s.pendHigh != 0 {
						combined := utf16.DecodeRune(s.pendHigh, r)
						s.appendRune(combined)
						s.pendHigh = 0
					}
					// lone low surrogate — skip silently
				default:
					if s.pendHigh != 0 {
						// dangling high surrogate — emit as-is then reset
						s.appendRune(s.pendHigh)
						s.pendHigh = 0
					}
					s.appendRune(r)
				}
			}
			s.hexBuf = s.hexBuf[:0]
			s.esc = escNone
		}
	}
}

// appendByte writes a decoded byte to (a) the current key's accumulator
// and (b) — only for watched keys — the streaming delta buffer.
func (s *JSONStringScanner) appendByte(c byte) {
	if s.curKey != "" {
		if acc := s.accumulators[s.curKey]; acc != nil {
			acc.buf = append(acc.buf, c)
		}
	}
	if !s.watching {
		return
	}
	s.deltaBuf = append(s.deltaBuf, c)
}

// appendRune writes a decoded rune's UTF-8 bytes to accumulator and
// (for watched keys) delta buffer.
func (s *JSONStringScanner) appendRune(r rune) {
	var buf [4]byte
	n := utf8.EncodeRune(buf[:], r)
	if s.curKey != "" {
		if acc := s.accumulators[s.curKey]; acc != nil {
			acc.buf = append(acc.buf, buf[:n]...)
		}
	}
	if !s.watching {
		return
	}
	s.deltaBuf = append(s.deltaBuf, buf[:n]...)
}

// flushDelta emits the accumulated decoded bytes for the current key,
// carrying any trailing incomplete UTF-8 bytes over to the next call.
func (s *JSONStringScanner) flushDelta() {
	if len(s.deltaBuf) == 0 {
		return
	}
	// Prepend any carried bytes from the previous flush.
	if len(s.carry) > 0 {
		merged := make([]byte, 0, len(s.carry)+len(s.deltaBuf))
		merged = append(merged, s.carry...)
		merged = append(merged, s.deltaBuf...)
		s.deltaBuf = merged
		s.carry = s.carry[:0]
	}
	full, rest := trimIncompleteUTF8(s.deltaBuf)
	if len(full) > 0 && s.onDelta != nil && s.curKey != "" {
		s.onDelta(s.curKey, string(full))
	}
	// If the string ended (state transitioned past stInString), there's no
	// next chunk for this key — emit the trailing bytes as-is and drop.
	if s.state != stInString && len(rest) > 0 && s.onDelta != nil && s.curKey != "" {
		s.onDelta(s.curKey, string(rest))
		rest = nil
	}
	s.carry = append(s.carry[:0], rest...)
	s.deltaBuf = s.deltaBuf[:0]
}

// trimIncompleteUTF8 returns the prefix of b that is a complete UTF-8
// sequence and the trailing bytes (if any) that form a partial sequence.
func trimIncompleteUTF8(b []byte) (full, rest []byte) {
	if len(b) == 0 {
		return b, nil
	}
	// Walk back at most 3 bytes to find the lead byte of the final rune.
	for i := len(b) - 1; i >= 0 && i >= len(b)-4; i-- {
		c := b[i]
		if c < 0x80 {
			// ASCII — prior bytes are complete, nothing to carry.
			return b, nil
		}
		if c&0xC0 == 0x80 {
			// continuation byte — keep walking back
			continue
		}
		// lead byte
		var need int
		switch {
		case c&0xE0 == 0xC0:
			need = 2
		case c&0xF0 == 0xE0:
			need = 3
		case c&0xF8 == 0xF0:
			need = 4
		default:
			return b, nil
		}
		have := len(b) - i
		if have >= need {
			return b, nil
		}
		return b[:i], b[i:]
	}
	return b, nil
}
