package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
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

// The transport must sort the tools on a clone and leave the caller's request
// untouched (net/http RoundTripper contract).
func TestAnthropicToolOrderTransport_ClonesWithoutMutatingOriginal(t *testing.T) {
	body := []byte(`{"max_tokens":1,"tools":[{"name":"charlie"},{"name":"alpha"},{"name":"bravo"}]}`)

	base := &stubRoundTripper{resp: newResp(200)}
	tr := &anthropicToolOrderTransport{base: base}

	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_ = resp.Body.Close()

	// Base receives a clone, not the caller's request, carrying sorted tools.
	if base.last == req {
		t.Fatal("base received the original request; want a clone")
	}
	sent, _ := io.ReadAll(base.last.Body)
	if got := toolNameOrder(t, sent); got != "alpha,bravo,charlie" {
		t.Errorf("sent tool order = %q, want sorted", got)
	}

	// Original request is untouched — its GetBody still yields the original order.
	rc, err := req.GetBody()
	if err != nil {
		t.Fatalf("original GetBody: %v", err)
	}
	orig, _ := io.ReadAll(rc)
	rc.Close()
	if got := toolNameOrder(t, orig); got != "charlie,alpha,bravo" {
		t.Errorf("original tool order = %q, want unchanged", got)
	}
}

func toolNameOrder(t *testing.T, body []byte) string {
	t.Helper()
	var top struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(body, &top); err != nil {
		t.Fatalf("unmarshal tools: %v", err)
	}
	names := make([]string, len(top.Tools))
	for i, tl := range top.Tools {
		names[i] = tl.Name
	}
	return strings.Join(names, ",")
}
