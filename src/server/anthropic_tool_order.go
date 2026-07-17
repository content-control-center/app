package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/ogen-app/ogen/src/logging"
)

var installAnthropicToolOrderOnce sync.Once

// InstallAnthropicToolOrderStabilizer wraps http.DefaultTransport so every
// outgoing Anthropic /v1/messages request has its `tools` array sorted by name
// before it is sent. This is the CON-112 fix.
//
// Root cause: the Genkit Anthropic plugin builds the outgoing tool list by
// ranging a Go map (firebase/genkit go/ai/generate.go), which yields a *random*
// order on every request, and it forces `strict: true` on every tool
// (plugins/internal/anthropic). Anthropic compiles strict tool schemas into a
// constrained-decoding grammar that is cached per exact tool-set, and the cache
// key is order-sensitive. A fresh random order therefore misses the cache on
// every call and pays the full server-side compile (~50s) each time — the
// chronic latency on the campaign_assistant and other tool-using flows.
//
// Imposing a deterministic order at the wire makes the cache warm after the
// first request (per ~24h TTL) and every subsequent request fast. The plugin
// builds its client from http.DefaultClient (→ http.DefaultTransport), so this
// intercepts every model call without forking the plugin — same hook point as
// InstallAnthropicHTTPLogging. Must be installed before the plugin builds its
// client. Idempotent; non-Anthropic traffic passes through untouched.
func InstallAnthropicToolOrderStabilizer() {
	installAnthropicToolOrderOnce.Do(func() {
		base := http.DefaultTransport
		if base == nil {
			base = &http.Transport{}
		}
		http.DefaultTransport = &anthropicToolOrderTransport{base: base}
		slog.Info("anthropic tool-order stabilizer installed", logging.AttrComponent, "anthropic.http")
	})
}

type anthropicToolOrderTransport struct{ base http.RoundTripper }

func (t *anthropicToolOrderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body == nil ||
		!strings.Contains(req.URL.Host, "anthropic") ||
		!strings.HasSuffix(req.URL.Path, "/messages") {
		return t.base.RoundTrip(req)
	}

	body := readAnthropicReqBody(req)
	if len(body) == 0 {
		return t.base.RoundTrip(req)
	}
	sorted, n, changed := sortAnthropicToolsByName(body)
	if !changed {
		return t.base.RoundTrip(req)
	}

	// RoundTrip must not modify the caller's request (net/http contract), so
	// rewrite the sorted body on a clone and leave the original untouched.
	clone := req.Clone(req.Context())
	setAnthropicReqBody(clone, sorted)
	slog.Debug("anthropic tools reordered for cache stability",
		logging.AttrComponent, "anthropic.http", "tools", n)
	return t.base.RoundTrip(clone)
}

// sortAnthropicToolsByName returns the body with its top-level `tools` array
// sorted by tool name. Only the tools array is reordered; every other top-level
// field is preserved as raw bytes, so the transform is lossless and
// deterministic across runs. changed is false when there is nothing to reorder
// (no tools, <2 tools, or a parse failure) so the original body is sent as-is.
func sortAnthropicToolsByName(body []byte) (out []byte, n int, changed bool) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil {
		return body, 0, false
	}
	raw, ok := top["tools"]
	if !ok {
		return body, 0, false
	}
	var tools []json.RawMessage
	if err := json.Unmarshal(raw, &tools); err != nil || len(tools) < 2 {
		return body, len(tools), false
	}
	sort.SliceStable(tools, func(i, j int) bool {
		return anthropicToolName(tools[i]) < anthropicToolName(tools[j])
	})
	newTools, err := json.Marshal(tools)
	if err != nil {
		return body, len(tools), false
	}
	top["tools"] = newTools
	rewritten, err := json.Marshal(top)
	if err != nil {
		return body, len(tools), false
	}
	return rewritten, len(tools), true
}

func anthropicToolName(raw json.RawMessage) string {
	var t struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &t)
	return t.Name
}

// readAnthropicReqBody returns a copy of the request body without mutating req.
// It reads through GetBody, which net/http populates for in-memory bodies such
// as the SDK's /v1/messages payload, so the caller's one-shot Body is never
// consumed. Returns nil when GetBody is unavailable — the caller then forwards
// the request untouched rather than draining the original body.
func readAnthropicReqBody(req *http.Request) []byte {
	if req.GetBody == nil {
		return nil
	}
	rc, err := req.GetBody()
	if err != nil {
		return nil
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil
	}
	return b
}

func setAnthropicReqBody(req *http.Request, body []byte) {
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
}
