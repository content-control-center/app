package post_assistant

import (
	"context"
	"testing"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/notes"
	"github.com/ogen-app/ogen/src/repository"
)

type captureNoteRepo struct {
	repository.PostNoteRepository
	created []*models.PostNote
}

func (r *captureNoteRepo) Create(_ context.Context, n *models.PostNote) error {
	r.created = append(r.created, n)
	return nil
}

func TestToolCreateNote(t *testing.T) {
	repo := &captureNoteRepo{}
	st := &requestState{postID: "post1", actor: "user1", noteSvc: notes.New(repo)}
	ctx := withRequestState(context.Background(), st)

	out, err := toolCreateNote(ctx, CreateNoteInput{Type: "image_prompt", Title: "Prompt", Body: "a photorealistic banana"})
	if err != nil {
		t.Fatalf("toolCreateNote: %v", err)
	}
	if out.ID == "" || out.Type != string(models.PostNoteTypeImagePrompt) {
		t.Errorf("unexpected output: %+v", out)
	}

	// draft_thesis is reserved for content-plan; the tool must downgrade it to
	// a free-form note rather than create a draft thesis.
	if _, err := toolCreateNote(ctx, CreateNoteInput{Type: "draft_thesis", Body: "should become note"}); err != nil {
		t.Fatalf("toolCreateNote (reserved type): %v", err)
	}

	if len(repo.created) != 2 {
		t.Fatalf("expected 2 notes persisted, got %d", len(repo.created))
	}
	first, second := repo.created[0], repo.created[1]
	if first.Type != models.PostNoteTypeImagePrompt {
		t.Errorf("first note type = %q, want image_prompt", first.Type)
	}
	if second.Type != models.PostNoteTypeNote {
		t.Errorf("reserved draft_thesis note type = %q, want note", second.Type)
	}
	for _, n := range repo.created {
		if n.Origin != models.PostNoteOriginAssistant {
			t.Errorf("origin = %q, want assistant", n.Origin)
		}
		if n.PostID != "post1" || n.CreatedBy != "user1" {
			t.Errorf("note not stamped with request state: %+v", n)
		}
	}
	if len(st.noteResults) != 2 {
		t.Errorf("noteResults = %d, want 2 (read by the runner to finalise the response)", len(st.noteResults))
	}
}

func TestToolCreateNote_Unavailable(t *testing.T) {
	st := &requestState{postID: "post1", actor: "user1"} // noteSvc nil
	ctx := withRequestState(context.Background(), st)
	if _, err := toolCreateNote(ctx, CreateNoteInput{Type: "note", Body: "x"}); err == nil {
		t.Fatal("expected an error when the note service is unavailable")
	}
}
