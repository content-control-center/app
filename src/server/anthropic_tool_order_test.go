package server

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSortAnthropicToolsByName(t *testing.T) {
	body := []byte(`{
		"max_tokens": 2048,
		"model": "claude-haiku-4-5-20251001",
		"system": [{"type":"text","text":"sys"}],
		"messages": [{"role":"user","content":"hi"}],
		"tools": [
			{"name":"charlie","description":"c","strict":true},
			{"name":"alpha","description":"a","strict":true},
			{"name":"bravo","description":"b","strict":true}
		]
	}`)

	out, n, changed := sortAnthropicToolsByName(body)
	if !changed || n != 3 {
		t.Fatalf("expected changed=true n=3, got changed=%v n=%d", changed, n)
	}

	// Tools are now in name order.
	var top struct {
		MaxTokens int               `json:"max_tokens"`
		Model     string            `json:"model"`
		Messages  []json.RawMessage `json:"messages"`
		Tools     []struct {
			Name   string `json:"name"`
			Strict bool   `json:"strict"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(out, &top); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	for i, w := range want {
		if top.Tools[i].Name != w {
			t.Errorf("tool[%d] = %q, want %q", i, top.Tools[i].Name, w)
		}
		if !top.Tools[i].Strict {
			t.Errorf("tool[%d] lost strict:true", i)
		}
	}

	// Non-tool fields preserved.
	if top.MaxTokens != 2048 || top.Model != "claude-haiku-4-5-20251001" || len(top.Messages) != 1 {
		t.Errorf("non-tool fields not preserved: %+v", top)
	}

	// Deterministic: re-running on already-sorted input yields identical bytes —
	// this is the property that keeps Anthropic's compilation cache warm.
	out2, _, _ := sortAnthropicToolsByName(out)
	if !bytes.Equal(out, out2) {
		t.Errorf("not idempotent/deterministic:\n first=%s\nsecond=%s", out, out2)
	}
}

func TestSortAnthropicToolsByName_NoOp(t *testing.T) {
	cases := map[string]string{
		"no tools field": `{"max_tokens":10,"messages":[]}`,
		"single tool":    `{"tools":[{"name":"only"}]}`,
		"empty tools":    `{"tools":[]}`,
		"not valid json": `not json at all`,
	}
	for name, in := range cases {
		out, _, changed := sortAnthropicToolsByName([]byte(in))
		if changed {
			t.Errorf("%s: expected changed=false", name)
		}
		if !bytes.Equal(out, []byte(in)) {
			t.Errorf("%s: body altered when it should pass through:\n in=%s\nout=%s", name, in, out)
		}
	}
}
