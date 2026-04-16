package content_plan

import (
	"strings"
	"testing"
)

func ptrInt(i int) *int { return &i }

func TestJoinPagedChunks_NoPageInfo(t *testing.T) {
	got := joinPagedChunks([]chunkPart{
		{Content: "alpha"},
		{Content: "beta"},
	})
	want := "alpha\n\n[...]\n\nbeta"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestJoinPagedChunks_SinglePage(t *testing.T) {
	got := joinPagedChunks([]chunkPart{
		{Content: "first chunk", PageStart: ptrInt(2), PageEnd: ptrInt(2)},
	})
	if !strings.HasPrefix(got, "[p. 2] ") {
		t.Errorf("expected single-page prefix, got %q", got)
	}
}

func TestJoinPagedChunks_PageRange(t *testing.T) {
	got := joinPagedChunks([]chunkPart{
		{Content: "pages three through five", PageStart: ptrInt(3), PageEnd: ptrInt(5)},
	})
	if !strings.HasPrefix(got, "[pp. 3–5] ") {
		t.Errorf("expected page-range prefix, got %q", got)
	}
}

func TestJoinPagedChunks_MixedPagesAndPlain(t *testing.T) {
	parts := []chunkPart{
		{Content: "intro"},
		{Content: "middle", PageStart: ptrInt(4), PageEnd: ptrInt(4)},
		{Content: "outro", PageStart: ptrInt(10), PageEnd: ptrInt(12)},
	}
	got := joinPagedChunks(parts)
	if !strings.Contains(got, "[p. 4] middle") {
		t.Errorf("missing single-page citation: %q", got)
	}
	if !strings.Contains(got, "[pp. 10–12] outro") {
		t.Errorf("missing page-range citation: %q", got)
	}
	// First part has no page info — should appear as-is, without a prefix.
	if !strings.HasPrefix(got, "intro\n\n") {
		t.Errorf("plain first part should be unprefixed: %q", got)
	}
}

func TestJoinPagedChunks_Empty(t *testing.T) {
	if got := joinPagedChunks(nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
