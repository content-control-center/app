package zernio

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// stub is the minimal route-table HTTP server reused across the
// post-related client tests. Same shape as the package's existing
// integration tests use elsewhere.
type stub struct {
	*httptest.Server
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
}

func newStub() *stub {
	s := &stub{routes: map[string]http.HandlerFunc{}}
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

func (s *stub) handle(method, path string, h http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[method+" "+path] = h
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func newClient(s *stub) *Client {
	return NewClient(StaticKey("key"), s.URL, ClientOpts{Timeout: 5 * time.Second})
}

func TestSubmitHappyPath(t *testing.T) {
	s := newStub()
	defer s.Close()

	s.handle("POST", "/posts", func(w http.ResponseWriter, r *http.Request) {
		var req SubmitRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Content != "hello" {
			t.Errorf("content: got %q want hello", req.Content)
		}
		writeJSON(w, http.StatusCreated, PostEnvelope{Post: Job{
			ID:     "post-1",
			Status: JobStatusScheduled,
		}})
	})

	c := newClient(s)
	job, err := c.Submit(context.Background(), SubmitRequest{Content: "hello", Platforms: []PlatformVariant{{Platform: "linkedin", AccountID: "acc1"}}})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if job.ID != "post-1" {
		t.Errorf("id: got %q want post-1", job.ID)
	}
	if job.Status != JobStatusScheduled {
		t.Errorf("status: got %q want scheduled", job.Status)
	}
}

func TestSubmit409ReturnsErrDuplicate(t *testing.T) {
	s := newStub()
	defer s.Close()
	s.handle("POST", "/posts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "duplicate content within 24h"})
	})
	c := newClient(s)
	_, err := c.Submit(context.Background(), SubmitRequest{Content: "x"})
	if !errors.Is(err, ErrDuplicateContent) {
		t.Fatalf("expected ErrDuplicateContent, got %v", err)
	}
}

func TestStatusReturnsTerminalEnum(t *testing.T) {
	s := newStub()
	defer s.Close()
	s.handle("GET", "/posts/abc", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, PostEnvelope{Post: Job{
			ID: "abc", Status: JobStatusPublished,
			Platforms: []PlatformOutcome{{Platform: "linkedin", Status: "published", PlatformPostURL: "https://linkedin.com/posts/x"}},
		}})
	})
	c := newClient(s)
	job, err := c.Status(context.Background(), "abc")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !job.Status.IsTerminal() {
		t.Errorf("expected terminal, got %q", job.Status)
	}
	if job.Platforms[0].PlatformPostURL == "" {
		t.Errorf("missing platformPostUrl")
	}
}

func TestCancelHappyPath(t *testing.T) {
	s := newStub()
	defer s.Close()
	s.handle("DELETE", "/posts/abc", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	c := newClient(s)
	if err := c.Cancel(context.Background(), "abc"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
}

func TestCancel404TreatedAsAlreadyPublished(t *testing.T) {
	s := newStub()
	defer s.Close()
	s.handle("DELETE", "/posts/gone", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	c := newClient(s)
	err := c.Cancel(context.Background(), "gone")
	if !errors.Is(err, ErrAlreadyPublished) {
		t.Fatalf("expected ErrAlreadyPublished, got %v", err)
	}
}

func TestCancel409TreatedAsAlreadyPublished(t *testing.T) {
	s := newStub()
	defer s.Close()
	s.handle("DELETE", "/posts/raced", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already published"})
	})
	c := newClient(s)
	err := c.Cancel(context.Background(), "raced")
	if !errors.Is(err, ErrAlreadyPublished) {
		t.Fatalf("expected ErrAlreadyPublished, got %v", err)
	}
}

func TestRetryHappyPath(t *testing.T) {
	s := newStub()
	defer s.Close()
	s.handle("POST", "/posts/abc/retry", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, PostEnvelope{Post: Job{ID: "abc", Status: JobStatusScheduled}})
	})
	c := newClient(s)
	job, err := c.Retry(context.Background(), "abc")
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if job.Status != JobStatusScheduled {
		t.Errorf("expected scheduled, got %q", job.Status)
	}
}

func TestFindByContentRecoversAfterDedupe(t *testing.T) {
	s := newStub()
	defer s.Close()
	s.handle("GET", "/posts", func(w http.ResponseWriter, r *http.Request) {
		// Verify the date filter went out (we don't pin the value).
		if r.URL.Query().Get("dateFrom") == "" {
			t.Error("dateFrom should be present")
		}
		writeJSON(w, http.StatusOK, listEnvelope{Posts: []Job{
			{ID: "stale", Content: "other"},
			{ID: "match", Content: "hello"},
		}})
	})
	c := newClient(s)
	job, err := c.FindByContent(context.Background(), "hello", time.Hour)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if job == nil || job.ID != "match" {
		t.Errorf("expected match, got %+v", job)
	}
}

func TestIsTerminalAPIError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{&APIError{Status: 400}, true},
		{&APIError{Status: 422}, true},
		{&APIError{Status: 429}, false},
		{&APIError{Status: 500}, false},
		{nil, false},
		{errors.New("network"), false},
	}
	for _, tc := range cases {
		if got := IsTerminalAPIError(tc.err); got != tc.want {
			t.Errorf("IsTerminalAPIError(%v): got %v want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsTransientAPIError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{&APIError{Status: 500}, true},
		{&APIError{Status: 429}, true},
		{&APIError{Status: 400}, false},
		{nil, false},
		{errors.New("network"), true},
	}
	for _, tc := range cases {
		if got := IsTransientAPIError(tc.err); got != tc.want {
			t.Errorf("IsTransientAPIError(%v): got %v want %v", tc.err, got, tc.want)
		}
	}
}

func TestSubmit400IsTerminal(t *testing.T) {
	// Sanity: a 400 from Submit is NOT auto-recovered, comes through as
	// a terminal APIError so the queue layer can resolve Failed.
	s := newStub()
	defer s.Close()
	s.handle("POST", "/posts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "platforms required"})
	})
	c := newClient(s)
	_, err := c.Submit(context.Background(), SubmitRequest{Content: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsTerminalAPIError(err) {
		t.Fatalf("expected terminal, got %v", err)
	}
	if !strings.Contains(err.Error(), "platforms required") {
		t.Errorf("error message should include zernio's reason: %v", err)
	}
}
