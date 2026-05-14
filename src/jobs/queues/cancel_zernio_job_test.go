package queues_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/content-control-center/app/src/jobs/queues"
	"github.com/content-control-center/app/src/models"
)

func TestCancelHappyPathTransitionsToReadyForPublish(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("DELETE", "/posts/z-1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	deps, postRepo, _ := makeDeps(stub, nil)
	post := seedScheduledPost(postRepo)
	post.ZernioPostID = "z-1"
	postRepo.put(post)

	proc := &queues.CancelZernioJobProcessor{Deps: deps}
	err := proc.Process(context.Background(), queues.CancelZernioJobTask{
		PostID: post.ID, Target: queues.CancelTargetReadyForPublish, Actor: "user-1",
	})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	got, _ := postRepo.GetByID(context.Background(), post.ID)
	if got.Status != models.PostStatusReadyForPublish {
		t.Errorf("status: got %q want ready_for_publish", got.Status)
	}
}

func TestCancelHappyPathTransitionsToDraft(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("DELETE", "/posts/z-1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	deps, postRepo, _ := makeDeps(stub, nil)
	post := seedScheduledPost(postRepo)
	post.ZernioPostID = "z-1"
	postRepo.put(post)

	proc := &queues.CancelZernioJobProcessor{Deps: deps}
	_ = proc.Process(context.Background(), queues.CancelZernioJobTask{
		PostID: post.ID, Target: queues.CancelTargetDraft, Actor: "user-1",
	})
	got, _ := postRepo.GetByID(context.Background(), post.ID)
	if got.Status != models.PostStatusDraft {
		t.Errorf("status: got %q want draft", got.Status)
	}
}

func TestCancelRaceWithPublishKeepsPostScheduled(t *testing.T) {
	// Zernio reports already-published; we MUST NOT transition the
	// post locally — the next poll tick will land Published.
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("DELETE", "/posts/z-1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already published"})
	})
	deps, postRepo, _ := makeDeps(stub, nil)
	post := seedScheduledPost(postRepo)
	post.ZernioPostID = "z-1"
	postRepo.put(post)

	proc := &queues.CancelZernioJobProcessor{Deps: deps}
	if err := proc.Process(context.Background(), queues.CancelZernioJobTask{
		PostID: post.ID, Target: queues.CancelTargetReadyForPublish, Actor: "user-1",
	}); err != nil {
		t.Fatalf("process should swallow race: %v", err)
	}
	got, _ := postRepo.GetByID(context.Background(), post.ID)
	if got.Status != models.PostStatusScheduled {
		t.Errorf("status: got %q want scheduled (race resolves via poll)", got.Status)
	}
}
