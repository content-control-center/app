package draft_post

// jsonObjScanner incrementally scans a stream of JSON text and yields complete
// top-level JSON objects (each element of the outer array) as they arrive. It is
// a copy of content_plan's private post scanner: the draft flow streams the same
// array-of-objects shape but stays decoupled from content_plan's internals.
type jsonObjScanner struct {
	buf      []byte
	depth    int
	inStr    bool
	escaped  bool
	objStart int // index of the opening '{' of the current object; -1 when not inside one
}

func newJSONObjScanner() *jsonObjScanner {
	return &jsonObjScanner{objStart: -1}
}

// push appends chunk to the internal buffer and returns all newly-complete JSON
// object strings found since the last call.
func (s *jsonObjScanner) push(chunk string) []string {
	var complete []string
	for i := 0; i < len(chunk); i++ {
		c := chunk[i]
		s.buf = append(s.buf, c)
		pos := len(s.buf) - 1

		if s.escaped {
			s.escaped = false
			continue
		}
		if s.inStr {
			switch c {
			case '\\':
				s.escaped = true
			case '"':
				s.inStr = false
			}
			continue
		}
		switch c {
		case '"':
			s.inStr = true
		case '{':
			if s.depth == 0 {
				s.objStart = pos
			}
			s.depth++
		case '}':
			s.depth--
			if s.depth == 0 && s.objStart >= 0 {
				complete = append(complete, string(s.buf[s.objStart:pos+1]))
				s.objStart = -1
			}
		}
	}
	return complete
}
