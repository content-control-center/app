package logs_test

import (
	"strings"
	"testing"

	"github.com/ogen-app/ogen/src/post_actions/logs"
)

func TestCapperPassesThroughSmallPayloads(t *testing.T) {
	in := `{"hello":"world"}`
	if got := logs.Capper(in); got != in {
		t.Errorf("expected pass-through, got %q", got)
	}
}

func TestCapperTruncatesOversizePayloads(t *testing.T) {
	big := strings.Repeat("x", logs.MaxPayloadBytes+1024)
	got := logs.Capper(big)
	if len(got) > logs.MaxPayloadBytes {
		t.Errorf("truncated payload still exceeds cap: %d > %d", len(got), logs.MaxPayloadBytes)
	}
	if !logs.ContainsTruncationMarker(got) {
		t.Errorf("missing truncation marker in %q", got[len(got)-80:])
	}
}

func TestSanitizeRedactsKnownKeys(t *testing.T) {
	in := `{"api_key":"sk-1234","password":"hunter2","other":"ok"}`
	got := logs.Sanitize(in)
	if strings.Contains(got, "sk-1234") || strings.Contains(got, "hunter2") {
		t.Errorf("leak: %q", got)
	}
	if !strings.Contains(got, `"other":"ok"`) {
		t.Errorf("non-secret fields should pass through: %q", got)
	}
}

func TestSanitizeRedactsBearerHeader(t *testing.T) {
	in := "Authorization: Bearer ABC123\nContent-Type: application/json"
	got := logs.Sanitize(in)
	if strings.Contains(got, "ABC123") {
		t.Errorf("leak: %q", got)
	}
}

func TestSanitizeAndCapAppliesBothInOrder(t *testing.T) {
	// Build a payload with a secret near the boundary; verify the
	// secret never survives, regardless of where the truncate cut falls.
	prefix := strings.Repeat("a", logs.MaxPayloadBytes-50)
	in := `{"token":"SECRET","pad":"` + prefix + `"}`
	got := logs.SanitizeAndCap(in)
	if strings.Contains(got, "SECRET") {
		t.Errorf("secret leaked through SanitizeAndCap: %q", got)
	}
}

func TestMarshalCappedHandlesUnmarshalable(t *testing.T) {
	got := logs.MarshalCapped(make(chan int))
	if !strings.Contains(got, "_error") {
		t.Errorf("expected fallback _error payload, got %q", got)
	}
}
