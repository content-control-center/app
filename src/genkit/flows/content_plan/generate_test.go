package content_plan

import (
	"context"
	"testing"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// CON-114: the streaming persist path stops at the batch's requested count, so
// an over-producing model can't turn "generate exactly 1" into 3 persisted
// posts. expectedCount <= 0 is the uncapped fallback (model decides the count).
func TestWithinCount(t *testing.T) {
	cases := []struct {
		name                string
		persisted, expected int
		want                bool
	}{
		{"first post for a 1-post batch", 0, 1, true},
		{"second post exceeds a 1-post batch", 1, 1, false},
		{"under a 3-post batch", 2, 3, true},
		{"at a 3-post batch cap", 3, 3, false},
		{"uncapped (zero)", 5, 0, true},
		{"uncapped (negative)", 5, -1, true},
	}
	for _, c := range cases {
		if got := withinCount(c.persisted, c.expected); got != c.want {
			t.Errorf("%s: withinCount(%d, %d) = %v, want %v", c.name, c.persisted, c.expected, got, c.want)
		}
	}
}

// stubPostRepo embeds the interface so it satisfies the type; only Create is
// implemented (the rest panic if unexpectedly called).
type stubPostRepo struct {
	repository.PostRepository
	created *models.Post
}

func (s *stubPostRepo) Create(_ context.Context, p *models.Post) error {
	s.created = p
	return nil
}

type stubNoteRepo struct {
	repository.PostNoteRepository
	created *models.PostNote
}

func (s *stubNoteRepo) Create(_ context.Context, n *models.PostNote) error {
	s.created = n
	return nil
}

// CON-188: content-plan no longer writes the thesis into the post body — the
// post is created with an empty body and the thesis is captured as a
// draft_thesis note (origin content_plan, authored by the campaign owner).
func TestPersistOne_DraftThesisNote(t *testing.T) {
	postRepo := &stubPostRepo{}
	noteRepo := &stubNoteRepo{}
	campaign := &models.Campaign{ID: "camp1", CreatedBy: "user1"}
	dp := DraftPost{Title: "T", Body: "- point 1\n- point 2", PlatformID: "linkedin", ContentType: "article"}

	id, err := persistOne(context.Background(), dp, campaign, postRepo, noteRepo, nil)
	if err != nil {
		t.Fatalf("persistOne: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty post id")
	}
	if postRepo.created == nil {
		t.Fatal("expected a post to be created")
	}
	if postRepo.created.Content != "" {
		t.Errorf("post body = %q, want empty (thesis goes to a note)", postRepo.created.Content)
	}
	if noteRepo.created == nil {
		t.Fatal("expected a draft_thesis note to be created")
	}
	n := noteRepo.created
	if n.Type != models.PostNoteTypeDraftThesis {
		t.Errorf("note type = %q, want draft_thesis", n.Type)
	}
	if n.Origin != models.PostNoteOriginContentPlan {
		t.Errorf("note origin = %q, want content_plan", n.Origin)
	}
	if n.Body != dp.Body {
		t.Errorf("note body = %q, want %q", n.Body, dp.Body)
	}
	if n.PostID != id {
		t.Errorf("note post_id = %q, want %q", n.PostID, id)
	}
	if n.CreatedBy != "user1" {
		t.Errorf("note created_by = %q, want user1", n.CreatedBy)
	}
}

// An empty thesis creates no note (but the post is still created).
func TestPersistOne_EmptyThesisNoNote(t *testing.T) {
	postRepo := &stubPostRepo{}
	noteRepo := &stubNoteRepo{}
	campaign := &models.Campaign{ID: "camp1", CreatedBy: "user1"}
	dp := DraftPost{Title: "T", Body: "   ", PlatformID: "linkedin", ContentType: "article"}

	if _, err := persistOne(context.Background(), dp, campaign, postRepo, noteRepo, nil); err != nil {
		t.Fatalf("persistOne: %v", err)
	}
	if postRepo.created == nil {
		t.Fatal("expected a post to be created")
	}
	if noteRepo.created != nil {
		t.Errorf("expected no note for an empty thesis, got %+v", noteRepo.created)
	}
}

// A nil note repo must not panic — the post is still created (note skipped).
func TestPersistOne_NilNoteRepo(t *testing.T) {
	postRepo := &stubPostRepo{}
	campaign := &models.Campaign{ID: "camp1", CreatedBy: "user1"}
	dp := DraftPost{Title: "T", Body: "- point 1", PlatformID: "linkedin", ContentType: "article"}

	if _, err := persistOne(context.Background(), dp, campaign, postRepo, nil, nil); err != nil {
		t.Fatalf("persistOne: %v", err)
	}
	if postRepo.created == nil {
		t.Fatal("expected a post to be created even with a nil note repo")
	}
}
