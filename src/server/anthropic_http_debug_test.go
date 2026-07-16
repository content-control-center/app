package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
)

// stubRoundTripper records the request it saw and returns a canned response.
type stubRoundTripper struct {
	called int
	last   *http.Request
	resp   *http.Response
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.called++
	s.last = req
	return s.resp, nil
}

func newResp(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
	}
}

// The wrapper must delegate to the base transport and return its response
// verbatim — it only logs and (for Anthropic hosts) attaches an httptrace,
// never altering the response body (so streaming is untouched).
func TestAnthropicLoggingTransport_PassThrough(t *testing.T) {
	cases := []struct {
		url    string
		traced bool // Anthropic hosts get an httptrace context ⇒ derived request
	}{
		{"https://api.anthropic.com/v1/messages", true},
		{"https://example.com/x", false},
	}
	for _, tc := range cases {
		base := &stubRoundTripper{resp: newResp(200)}
		tr := &anthropicLoggingTransport{base: base}

		req := httptest.NewRequest(http.MethodPost, tc.url, nil)
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.url, err)
		}
		if base.called != 1 {
			t.Errorf("%s: base called %d times, want 1", tc.url, base.called)
		}
		if resp != base.resp {
			t.Errorf("%s: wrapper did not return the base response verbatim", tc.url)
		}
		// Non-Anthropic requests pass through untouched; Anthropic requests are
		// forwarded as a context-derived copy (same URL + method) carrying a trace.
		if tc.traced {
			if base.last == req {
				t.Errorf("%s: expected a context-derived request, got the original", tc.url)
			}
			if base.last.URL.String() != req.URL.String() || base.last.Method != req.Method {
				t.Errorf("%s: derived request altered URL/method", tc.url)
			}
			if httptrace.ContextClientTrace(base.last.Context()) == nil {
				t.Errorf("%s: expected an httptrace on the forwarded request", tc.url)
			}
		} else if base.last != req {
			t.Errorf("%s: wrapper did not forward the original request unchanged", tc.url)
		}
	}
}
