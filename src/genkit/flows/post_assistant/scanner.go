package post_assistant

import (
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// JSONStringScanner is an incremental parser for a single top-level JSON
// object. It watches a set of top-level string keys and invokes onDelta
// with a decoded fragment each time new bytes of a watched value arrive.
// Non-watched keys and non-string values are skipped without emission.
// The raw text is also buffered so the complete JSON can be retrieved and
// unmarshaled once the stream finishes.
//
// The scanner is forgiving of preamble (e.g. markdown fences) before the
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

	// fullText accumulates the raw input bytes for final unmarshaling.
	fullText []byte
}

type scannerState int

const (
	stPreamble scannerState = iota
	stTop                   // between keys at the top level
	stInKey
	stAfterKey
	stAwaitValue
	stInString
	stInLiteral
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
// decoded bytes of a watched string key are available.
func NewJSONStringScanner(watched []string, onDelta func(key, delta string)) *JSONStringScanner {
	m := make(map[string]bool, len(watched))
	for _, k := range watched {
		m[k] = true
	}
	return &JSONStringScanner{watched: m, onDelta: onDelta}
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
			s.state = stInString
		case '{', '[':
			s.nestedDepth = 1
			s.state = stInNested
		default:
			// bool / number / null literal
			s.state = stInLiteral
		}
	case stInString:
		s.stringByte(c)
	case stInLiteral:
		switch c {
		case ',':
			s.resetKey()
			s.state = stTop
		case '}':
			s.state = stDone
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
			s.state = stInLiteral // drives the comma/brace skip
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

func (s *JSONStringScanner) appendByte(c byte) {
	if !s.watching {
		return
	}
	s.deltaBuf = append(s.deltaBuf, c)
}

func (s *JSONStringScanner) appendRune(r rune) {
	if !s.watching {
		return
	}
	var buf [4]byte
	n := utf8.EncodeRune(buf[:], r)
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
