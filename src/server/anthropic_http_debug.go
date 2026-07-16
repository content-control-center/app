package server

import (
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"

	"github.com/ogen-app/ogen/src/logging"
)

var installAnthropicHTTPLoggingOnce sync.Once

// InstallAnthropicHTTPLogging wraps http.DefaultTransport so every Anthropic
// round-trip is logged, split into httptrace phases — connection acquisition
// (conn_ms) vs. the server's own time-to-first-byte (server_ms). The genkit
// Anthropic plugin builds its client from http.DefaultClient (→
// http.DefaultTransport), so this intercepts every model call without forking
// the plugin. Non-Anthropic traffic passes through untouched.
//
// Opt-in via ANTHROPIC_DEBUG_HTTP; idempotent. Diagnostic only — this is how the
// ~50-60s per-request latency was localized to server-side generation (large
// server_ms with conn_ms≈0), then traced by diffing the outgoing bytes to
// strict-tool-schema recompilation caused by a per-request random tool order.
// The fix is InstallAnthropicToolOrderStabilizer (see anthropic_tool_order.go).
func InstallAnthropicHTTPLogging() {
	installAnthropicHTTPLoggingOnce.Do(func() {
		base := http.DefaultTransport
		if base == nil {
			base = &http.Transport{}
		}
		http.DefaultTransport = &anthropicLoggingTransport{base: base}
		slog.Info("anthropic http logging installed", logging.AttrComponent, "anthropic.http")
	})
}

type anthropicLoggingTransport struct{ base http.RoundTripper }

func (t *anthropicLoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only touch Anthropic traffic; everything else passes straight through.
	if !strings.Contains(req.URL.Host, "anthropic") {
		return t.base.RoundTrip(req)
	}
	return t.roundTripLogged(req)
}

func (t *anthropicLoggingTransport) roundTripLogged(req *http.Request) (*http.Response, error) {
	start := time.Now()
	// Per-request timestamps — each call gets its own closures, so this is
	// goroutine-safe under concurrent requests.
	var (
		tGetConn, tGotConn, tDNSDone, tConnDone, tTLSDone, tWroteReq, tFirstByte time.Time
		reused, wasIdle                                                          bool
		idle                                                                     time.Duration
	)
	trace := &httptrace.ClientTrace{
		GetConn: func(string) { tGetConn = time.Now() },
		GotConn: func(i httptrace.GotConnInfo) {
			tGotConn = time.Now()
			reused = i.Reused
			wasIdle = i.WasIdle
			idle = i.IdleTime
		},
		DNSDone:              func(httptrace.DNSDoneInfo) { tDNSDone = time.Now() },
		ConnectDone:          func(string, string, error) { tConnDone = time.Now() },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { tTLSDone = time.Now() },
		WroteRequest:         func(httptrace.WroteRequestInfo) { tWroteReq = time.Now() },
		GotFirstResponseByte: func() { tFirstByte = time.Now() },
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	resp, err := t.base.RoundTrip(req)
	total := time.Since(start)

	ms := func(at time.Time) int64 {
		if at.IsZero() {
			return -1
		}
		return at.Sub(start).Milliseconds()
	}
	serverMS := int64(-1)
	if !tWroteReq.IsZero() && !tFirstByte.IsZero() {
		serverMS = tFirstByte.Sub(tWroteReq).Milliseconds()
	}

	attrs := []any{
		logging.AttrComponent, "anthropic.http",
		"method", req.Method,
		"path", req.URL.Path,
		"total_ms", total.Milliseconds(),
		"conn_ms", ms(tGotConn), // ← large ⇒ blocked acquiring a connection
		"server_ms", serverMS, // ← large ⇒ request-written→first-byte (the API/edge)
		"get_conn_ms", ms(tGetConn),
		"dns_done_ms", ms(tDNSDone),
		"connect_done_ms", ms(tConnDone),
		"tls_done_ms", ms(tTLSDone),
		"wrote_req_ms", ms(tWroteReq),
		"first_byte_ms", ms(tFirstByte),
		"conn_reused", reused,
		"conn_was_idle", wasIdle,
		"conn_idle_ms", idle.Milliseconds(),
	}
	if err != nil {
		attrs = append(attrs, logging.AttrError, err)
		slog.Warn("anthropic http attempt", attrs...)
		return resp, err
	}
	attrs = append(attrs,
		"status", resp.StatusCode,
		"proto", resp.Proto,
		"retry_after", resp.Header.Get("retry-after"),
		"request_id", resp.Header.Get("request-id"),
		"req_remaining", resp.Header.Get("anthropic-ratelimit-requests-remaining"),
		"input_tok_remaining", resp.Header.Get("anthropic-ratelimit-input-tokens-remaining"),
		"output_tok_remaining", resp.Header.Get("anthropic-ratelimit-output-tokens-remaining"),
	)
	slog.Info("anthropic http attempt", attrs...)
	return resp, err
}
