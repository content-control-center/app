package queues_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/jobs/queues"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/repository"
)

// fakePostRepo is the narrow surface the queue handlers actually
// touch. A real repository.PostRepository is overkill for unit tests
// of the queue; keeping a tiny in-memory shim makes failure modes
// obvious.
type fakePostRepo struct {
	mu    sync.Mutex
	posts map[string]*models.Post
}

func newFakePostRepo() *fakePostRepo { return &fakePostRepo{posts: map[string]*models.Post{}} }

func (r *fakePostRepo) put(p *models.Post) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *p
	r.posts[p.ID] = &cp
}
func (r *fakePostRepo) GetByID(_ context.Context, id string) (*models.Post, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.posts[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, errors.New("not found")
}
func (r *fakePostRepo) Update(_ context.Context, p *models.Post) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *p
	r.posts[p.ID] = &cp
	return nil
}

// Stubs for the rest of the PostRepository surface — never called by
// the queues but required to satisfy the interface. We type-assert in
// the test setup so any forgotten method becomes a compile error.
func (r *fakePostRepo) List(context.Context) ([]models.Post, error) { return nil, nil }
func (r *fakePostRepo) ListByCampaign(context.Context, string) ([]models.Post, error) {
	return nil, nil
}
func (r *fakePostRepo) ListWithPublisherPostID(context.Context) ([]models.Post, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []models.Post
	for _, p := range r.posts {
		if p.Publisher == models.PublisherZernio && p.PublisherPostID != "" {
			out = append(out, models.Post{ID: p.ID, PublisherPostID: p.PublisherPostID})
		}
	}
	return out, nil
}
func (r *fakePostRepo) Create(context.Context, *models.Post) error        { return nil }
func (r *fakePostRepo) CreateBatch(context.Context, []*models.Post) error { return nil }
func (r *fakePostRepo) Delete(context.Context, string) (bool, error)      { return false, nil }
func (r *fakePostRepo) ListStuckScheduled(context.Context, time.Time, int) ([]models.Post, error) {
	return nil, nil
}
func (r *fakePostRepo) UpdateStatusAndReason(_ context.Context, id string, st models.PostStatus, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.posts[id]; ok {
		p.Status = st
		p.FailureReason = reason
	}
	return nil
}

// fakeLogRepo records every PostLog Append for assertions.
type fakeLogRepo struct {
	mu      sync.Mutex
	entries []*models.PostLog
}

func newFakeLogRepo() *fakeLogRepo { return &fakeLogRepo{} }
func (r *fakeLogRepo) Append(_ context.Context, entry *models.PostLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *entry
	r.entries = append(r.entries, &cp)
	return nil
}
func (r *fakeLogRepo) AppendTx(ctx context.Context, _ bun.IDB, e *models.PostLog) error {
	return r.Append(ctx, e)
}
func (r *fakeLogRepo) ListByPostID(context.Context, string, int) ([]models.PostLog, error) {
	return nil, nil
}
func (r *fakeLogRepo) ListFiltered(context.Context, repository.PostLogFilter) ([]models.PostLog, error) {
	return nil, nil
}
func (r *fakeLogRepo) DeleteOlderThan(context.Context, time.Time) (int64, error) { return 0, nil }
func (r *fakeLogRepo) eventTypes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, string(e.EventType))
	}
	return out
}

// fakeAccountRepo returns a fixed list per profile.
type fakeAccountRepo struct {
	accounts map[string][]models.SocialAccount
}

func (r *fakeAccountRepo) ListAll(context.Context, string) ([]models.SocialAccount, error) {
	return nil, nil
}
func (r *fakeAccountRepo) ListActive(_ context.Context, profileID string) ([]models.SocialAccount, error) {
	return r.accounts[profileID], nil
}
func (r *fakeAccountRepo) ApplyPlan(context.Context, []models.SocialAccount, []string, time.Time) error {
	return nil
}

// stubZernio is the route-table HTTP server reused across queue tests.
type stubZernio struct {
	*httptest.Server
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
}

func newStubZernio() *stubZernio {
	s := &stubZernio{routes: map[string]http.HandlerFunc{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		h, ok := s.routes[r.Method+" "+r.URL.Path]
		s.mu.Unlock()
		if !ok {
			http.Error(w, "stub: unhandled "+r.Method+" "+r.URL.Path, http.StatusNotFound)
			return
		}
		h(w, r)
	}))
	return s
}

func (s *stubZernio) handle(method, path string, h http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[method+" "+path] = h
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func makeDeps(stub *stubZernio, accounts map[string][]models.SocialAccount) (queues.ZernioDeps, *fakePostRepo, *fakeLogRepo) {
	postRepo := newFakePostRepo()
	logRepo := newFakeLogRepo()
	deps := queues.ZernioDeps{
		PostRepo:          postRepo,
		PostLogRepo:       logRepo,
		SocialAccountRepo: &fakeAccountRepo{accounts: accounts},
		Client:            zernio.NewClient(zernio.StaticKey("k"), stub.URL, zernio.ClientOpts{Timeout: 5 * time.Second}),
		ProfileID:         func(context.Context) (string, error) { return "p_test", nil },
	}
	return deps, postRepo, logRepo
}

func seedScheduledPost(repo *fakePostRepo) *models.Post {
	now := time.Now().Add(-time.Minute).UTC()
	post := &models.Post{
		ID:          "post-1",
		PlatformID:  "AXqWG7U2qnpt", // LinkedIn Sqid
		Content:     "hello world",
		Status:      models.PostStatusScheduled,
		ScheduledAt: &now,
		Platform: &models.Platform{
			ID:   "AXqWG7U2qnpt",
			Name: "LinkedIn",
		},
	}
	repo.put(post)
	return post
}

func TestSubmitHappyPathPersistsZernioID(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("POST", "/posts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusCreated, zernio.PostEnvelope{Post: zernio.Job{
			ID: "z-1", Status: zernio.JobStatusScheduled,
		}})
	})
	deps, postRepo, logRepo := makeDeps(stub, map[string][]models.SocialAccount{
		"p_test": {{ID: "acc-1", Platform: "linkedin"}},
	})
	post := seedScheduledPost(postRepo)

	proc := &queues.SubmitPostProcessor{Deps: deps}
	if err := proc.Process(context.Background(), queues.SubmitPostTask{PostID: post.ID}); err != nil {
		t.Fatalf("process: %v", err)
	}
	got, _ := postRepo.GetByID(context.Background(), post.ID)
	if got.PublisherPostID != "z-1" {
		t.Errorf("zernio_post_id: got %q want z-1", got.PublisherPostID)
	}
	events := logRepo.eventTypes()
	hasSubmit := false
	for _, e := range events {
		if e == string(models.PostLogEventZernioSubmit) {
			hasSubmit = true
		}
	}
	if !hasSubmit {
		t.Errorf("expected zernio_submit event, got %v", events)
	}
}

func TestSubmitTerminalRejectionMovesPostToFailed(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("POST", "/posts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "platforms required"})
	})
	deps, postRepo, _ := makeDeps(stub, map[string][]models.SocialAccount{
		"p_test": {{ID: "acc-1", Platform: "linkedin"}},
	})
	post := seedScheduledPost(postRepo)

	proc := &queues.SubmitPostProcessor{Deps: deps}
	if err := proc.Process(context.Background(), queues.SubmitPostTask{PostID: post.ID}); err != nil {
		t.Fatalf("process should swallow terminal err: %v", err)
	}
	got, _ := postRepo.GetByID(context.Background(), post.ID)
	if got.Status != models.PostStatusFailed {
		t.Errorf("status: got %q want failed", got.Status)
	}
	if got.FailureReason == "" {
		t.Error("failure_reason should be set")
	}
}

func TestSubmitTransientErrorBubblesForRetry(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("POST", "/posts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "later"})
	})
	deps, postRepo, _ := makeDeps(stub, map[string][]models.SocialAccount{
		"p_test": {{ID: "acc-1", Platform: "linkedin"}},
	})
	post := seedScheduledPost(postRepo)

	proc := &queues.SubmitPostProcessor{Deps: deps}
	err := proc.Process(context.Background(), queues.SubmitPostTask{PostID: post.ID})
	if err == nil {
		t.Fatal("expected non-nil error so backlite retries")
	}
	got, _ := postRepo.GetByID(context.Background(), post.ID)
	if got.Status != models.PostStatusScheduled {
		t.Errorf("status: got %q want scheduled (no transition on transient)", got.Status)
	}
}

func TestSubmitDedupeRecoveryAdoptsExistingJob(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("POST", "/posts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "duplicate within 24h"})
	})
	stub.handle("GET", "/posts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"posts": []zernio.Job{{ID: "z-existing", Content: "hello world"}},
		})
	})
	deps, postRepo, _ := makeDeps(stub, map[string][]models.SocialAccount{
		"p_test": {{ID: "acc-1", Platform: "linkedin"}},
	})
	post := seedScheduledPost(postRepo)

	proc := &queues.SubmitPostProcessor{Deps: deps}
	if err := proc.Process(context.Background(), queues.SubmitPostTask{PostID: post.ID}); err != nil {
		t.Fatalf("process: %v", err)
	}
	got, _ := postRepo.GetByID(context.Background(), post.ID)
	if got.PublisherPostID != "z-existing" {
		t.Errorf("zernio_post_id: got %q want z-existing", got.PublisherPostID)
	}
}

func TestSubmitNoAccountConnectedFails(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	deps, postRepo, _ := makeDeps(stub, map[string][]models.SocialAccount{
		"p_test": {}, // no accounts
	})
	post := seedScheduledPost(postRepo)

	proc := &queues.SubmitPostProcessor{Deps: deps}
	if err := proc.Process(context.Background(), queues.SubmitPostTask{PostID: post.ID}); err != nil {
		t.Fatalf("process should swallow terminal: %v", err)
	}
	got, _ := postRepo.GetByID(context.Background(), post.ID)
	if got.Status != models.PostStatusFailed {
		t.Errorf("status: got %q want failed", got.Status)
	}
}
