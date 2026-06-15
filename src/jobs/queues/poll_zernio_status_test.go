package queues_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ogen-app/ogen/src/jobs/queues"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/publishers/zernio"
)

func TestPollTerminalPublishedTransitionsPost(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("GET", "/posts/z-1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, zernio.PostEnvelope{Post: zernio.Job{
			ID: "z-1", Status: zernio.JobStatusPublished,
			Platforms: []zernio.PlatformOutcome{{Platform: "linkedin", Status: "published", PlatformPostURL: "https://li/x"}},
		}})
	})
	deps, postRepo, logRepo := makeDeps(stub, nil)
	post := seedScheduledPost(postRepo)
	post.PublisherPostID = "z-1"
	postRepo.put(post)

	proc := &queues.PollZernioStatusProcessor{Deps: deps}
	if err := proc.Process(context.Background(), queues.PollZernioStatusTask{PostID: post.ID}); err != nil {
		t.Fatalf("process: %v", err)
	}
	got, _ := postRepo.GetByID(context.Background(), post.ID)
	if got.Status != models.PostStatusPublished {
		t.Errorf("status: got %q want published", got.Status)
	}
	if got.PublishedAt == nil {
		t.Error("published_at should be set")
	}
	if got.PublishedResults == "" {
		t.Error("published_results should be set with platform outcomes")
	}
	_ = logRepo
}

func TestPollTerminalFailedTransitionsPost(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("GET", "/posts/z-1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, zernio.PostEnvelope{Post: zernio.Job{
			ID: "z-1", Status: zernio.JobStatusFailed,
		}})
	})
	deps, postRepo, _ := makeDeps(stub, nil)
	post := seedScheduledPost(postRepo)
	post.PublisherPostID = "z-1"
	postRepo.put(post)

	proc := &queues.PollZernioStatusProcessor{Deps: deps}
	if err := proc.Process(context.Background(), queues.PollZernioStatusTask{PostID: post.ID}); err != nil {
		t.Fatalf("process: %v", err)
	}
	got, _ := postRepo.GetByID(context.Background(), post.ID)
	if got.Status != models.PostStatusFailed {
		t.Errorf("status: got %q want failed", got.Status)
	}
}

func TestPollPartialMapsToFailed(t *testing.T) {
	// CON-69 deferred PartiallyPublished, so partial → Failed for now
	// with the per-platform breakdown captured in PostLog/results.
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("GET", "/posts/z-1", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, zernio.PostEnvelope{Post: zernio.Job{
			ID: "z-1", Status: zernio.JobStatusPartial,
			Platforms: []zernio.PlatformOutcome{
				{Platform: "linkedin", Status: "published"},
				{Platform: "twitter", Status: "failed", ErrorMessage: "auth"},
			},
		}})
	})
	deps, postRepo, _ := makeDeps(stub, nil)
	post := seedScheduledPost(postRepo)
	post.PublisherPostID = "z-1"
	postRepo.put(post)

	proc := &queues.PollZernioStatusProcessor{Deps: deps}
	_ = proc.Process(context.Background(), queues.PollZernioStatusTask{PostID: post.ID})
	got, _ := postRepo.GetByID(context.Background(), post.ID)
	if got.Status != models.PostStatusFailed {
		t.Errorf("status: got %q want failed (partial → Failed for MVP)", got.Status)
	}
}

func TestPollExitsCleanlyWhenPostNotScheduled(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	// No route registered → would 500 if the handler called Status.
	deps, postRepo, _ := makeDeps(stub, nil)
	post := seedScheduledPost(postRepo)
	post.Status = models.PostStatusFailed // user/reconciler already moved it
	postRepo.put(post)

	proc := &queues.PollZernioStatusProcessor{Deps: deps}
	if err := proc.Process(context.Background(), queues.PollZernioStatusTask{PostID: post.ID}); err != nil {
		t.Fatalf("process should return nil for non-scheduled posts: %v", err)
	}
}
