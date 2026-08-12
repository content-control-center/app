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
