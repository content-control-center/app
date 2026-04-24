package post_assistant

// stripTrailingCommas removes "," tokens that sit between an array/object
// value and its closing "]" or "}" — a recurring Claude output quirk under
// JSON-schema constraint. The scan is quote-aware so commas inside string
// values are preserved; backslash escapes inside strings are honoured so
// sequences like "\"" don't falsely end a string.
//
// Whitespace between the trailing comma and the closing bracket is kept.
func stripTrailingCommas(s string) string {
	b := []byte(s)
	out := make([]byte, 0, len(b))
	inStr := false
	escaped := false
	commaIdx := -1 // index in out of the most recent comma at structural level

	for i := 0; i < len(b); i++ {
		c := b[i]
		if inStr {
			out = append(out, c)
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
			out = append(out, c)
			commaIdx = -1
		case ',':
			commaIdx = len(out)
			out = append(out, c)
		case '}', ']':
			if commaIdx >= 0 {
				out = append(out[:commaIdx], out[commaIdx+1:]...)
			}
			out = append(out, c)
			commaIdx = -1
		case ' ', '\t', '\n', '\r':
			out = append(out, c)
			// Whitespace between comma and closer is fine — keep commaIdx.
		default:
			out = append(out, c)
			commaIdx = -1
		}
	}
	return string(out)
}
