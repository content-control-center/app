package post_assistant

import (
	"context"
	"strings"
	"testing"
)

// The editPost tool must reject an empty instruction before it ever spins up
// the writer sub-call, and must not record an edit result.
func TestToolEditPost_RequiresInstruction(t *testing.T) {
	st := &requestState{postID: "p1"}
	ctx := withRequestState(context.Background(), st)
	if _, err := toolEditPost(ctx, EditPostInput{Instruction: "   "}); err == nil {
		t.Fatal("expected an error for an empty instruction")
	}
	if st.editResult != nil {
		t.Fatal("editResult must stay nil when no writer ran")
	}
}

// runWriter is a no-op in the legacy state (no genkit instance / provider /
// writer system prompt) — the guard keeps a stray call from panicking and
// clearly reports the writer is unavailable.
func TestRunWriter_Unavailable(t *testing.T) {
	st := &requestState{postID: "p1"} // g / provider / writerSystem all zero
	if _, err := runWriter(context.Background(), st, "shorten it", false); err == nil {
		t.Fatal("expected an error when the writer is unavailable")
	}
}

// A well-formed editPost call whose writer is unavailable surfaces a wrapped
// write-content error and leaves editResult unset, so the runner never
// finalises a bogus "edited" turn.
func TestToolEditPost_WriterUnavailable(t *testing.T) {
	st := &requestState{postID: "p1"} // writerSystem empty → runWriter errors
	ctx := withRequestState(context.Background(), st)
	_, err := toolEditPost(ctx, EditPostInput{Instruction: "make it punchier"})
	if err == nil {
		t.Fatal("expected an error when content writing is unavailable")
	}
	if !strings.Contains(err.Error(), "write content") {
		t.Fatalf("expected a wrapped write-content error, got: %v", err)
	}
	if st.editResult != nil {
		t.Fatal("editResult must stay nil on writer failure")
	}
}

// The writer must receive the full retrieved excerpts as source material —
// alongside the unchanged, verbatim instruction — so an asset-grounded edit
// grounds on the retrieved text rather than the short preview (CON-128).
func TestComposeWriterInstruction_IncludesRetrievedExcerpts(t *testing.T) {
	instruction := "Add a section on goroutine scheduling from the whitepaper."
	excerpts := []retrievedExcerpt{
		{AssetID: "asset1", ChunkID: "c1", Content: "Goroutines are multiplexed onto OS threads by the Go runtime scheduler."},
	}

	out := composeWriterInstruction(instruction, excerpts)

	if !strings.HasPrefix(out, instruction) {
		t.Fatalf("the verbatim instruction must lead the writer prompt; got: %q", out)
	}
	if !strings.Contains(out, "multiplexed onto OS threads") {
		t.Fatalf("the retrieved excerpt content must reach the writer as source material; got: %q", out)
	}
	if !strings.Contains(out, "Source material") {
		t.Fatalf("excerpts should be labelled as source material; got: %q", out)
	}

	// No retrieval this turn → the instruction is passed through untouched.
	if got := composeWriterInstruction(instruction, nil); got != instruction {
		t.Fatalf("with no excerpts the instruction must be unchanged; got: %q", got)
	}
}

// Asset-retrieval tools may return the same chunk more than once across a turn;
// captureExcerpts must record each chunk once so the writer prompt isn't padded
// with duplicates.
func TestCaptureExcerpts_DedupesByChunkID(t *testing.T) {
	st := &requestState{}
	out := &ChunksOutput{Chunks: []ChunkContent{
		{ID: "c1", Content: "alpha"},
		{ID: "c2", Content: "beta"},
	}}

	st.captureExcerpts("a1", out)
	st.captureExcerpts("a1", out) // same chunks retrieved again

	if len(st.retrieved) != 2 {
		t.Fatalf("expected 2 deduped excerpts, got %d", len(st.retrieved))
	}
	if st.retrieved[0].Content != "alpha" || st.retrieved[1].Content != "beta" {
		t.Fatalf("captured excerpts lost their content: %+v", st.retrieved)
	}
}
