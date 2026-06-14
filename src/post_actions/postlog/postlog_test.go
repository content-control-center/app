package postlog_test

import (
	"strings"
	"testing"

	"github.com/ogen-app/ogen/src/post_actions/postlog"
)

func TestCapperPassesThroughSmallPayloads(t *testing.T) {
	in := `{"hello":"world"}`
	if got := postlog.Capper(in); got != in {
		t.Errorf("expected pass-through, got %q", got)
	}
}

func TestCapperTruncatesOversizePayloads(t *testing.T) {
	big := strings.Repeat("x", postlog.MaxPayloadBytes+1024)
	got := postlog.Capper(big)
	if len(got) > postlog.MaxPayloadBytes {
		t.Errorf("truncated payload still exceeds cap: %d > %d", len(got), postlog.MaxPayloadBytes)
	}
	if !postlog.ContainsTruncationMarker(got) {
		t.Errorf("missing truncation marker in %q", got[len(got)-80:])
	}
}

func TestSanitizeRedactsKnownKeys(t *testing.T) {
	in := `{"api_key":"sk-1234","password":"hunter2","other":"ok"}`
	got := postlog.Sanitize(in)
	if strings.Contains(got, "sk-1234") || strings.Contains(got, "hunter2") {
		t.Errorf("leak: %q", got)
	}
	if !strings.Contains(got, `"other":"ok"`) {
		t.Errorf("non-secret fields should pass through: %q", got)
	}
}

func TestSanitizeRedactsBearerHeader(t *testing.T) {
	in := "Authorization: Bearer ABC123\nContent-Type: application/json"
	got := postlog.Sanitize(in)
	if strings.Contains(got, "ABC123") {
		t.Errorf("leak: %q", got)
	}
}

func TestSanitizeAndCapAppliesBothInOrder(t *testing.T) {
	// Build a payload with a secret near the boundary; verify the
	// secret never survives, regardless of where the truncate cut falls.
	prefix := strings.Repeat("a", postlog.MaxPayloadBytes-50)
	in := `{"token":"SECRET","pad":"` + prefix + `"}`
	got := postlog.SanitizeAndCap(in)
	if strings.Contains(got, "SECRET") {
		t.Errorf("secret leaked through SanitizeAndCap: %q", got)
	}
}

func TestMarshalCappedHandlesUnmarshalable(t *testing.T) {
	got := postlog.MarshalCapped(make(chan int))
	if !strings.Contains(got, "_error") {
		t.Errorf("expected fallback _error payload, got %q", got)
	}
}
