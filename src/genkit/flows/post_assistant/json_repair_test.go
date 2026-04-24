package post_assistant

import "testing"

func TestStripTrailingCommas(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no change on clean object", `{"a":"b","c":1}`, `{"a":"b","c":1}`},
		{"trailing comma in object", `{"a":"b","c":1,}`, `{"a":"b","c":1}`},
		{"trailing comma with whitespace", "{\"a\":\"b\",\n}", "{\"a\":\"b\"\n}"},
		{"trailing comma in array", `[1,2,3,]`, `[1,2,3]`},
		{"multiple trailing commas in nested", `{"a":[1,2,],"b":{"x":"y",}}`, `{"a":[1,2],"b":{"x":"y"}}`},
		{"comma inside string preserved", `{"msg":"hello, world",}`, `{"msg":"hello, world"}`},
		{"escaped quote inside string not confused for end", `{"a":"say \"hi, there\"",}`, `{"a":"say \"hi, there\""}`},
		{"escaped backslash inside string", `{"a":"back\\\\slash",}`, `{"a":"back\\\\slash"}`},
		{"brace inside string", `{"a":"look { at this }","b":1,}`, `{"a":"look { at this }","b":1}`},
		{"empty object", `{}`, `{}`},
		{"empty array", `[]`, `[]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripTrailingCommas(tt.in)
			if got != tt.want {
				t.Errorf("stripTrailingCommas(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
		})
	}
}
