package jobs_test

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mikestefanello/backlite"
	_ "github.com/ncruces/go-sqlite3/driver"

	"github.com/content-control-center/app/src/jobs"
)

// noopTask is a marker payload used by the smoke test below.
type noopTask struct {
	Marker string `json:"marker"`
}

func (noopTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{
		Name:    "noop_smoke",
		Timeout: 5 * time.Second,
		Backoff: 50 * time.Millisecond,
	}
}

// panicTask exercises the built-in dispatcher recovery so a panicking
// handler doesn't crash the worker pool.
type panicTask struct{}

func (panicTask) Config() backlite.QueueConfig {
	return backlite.QueueConfig{
		Name:        "panic_smoke",
		MaxAttempts: 1,
		Timeout:     5 * time.Second,
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	// Anonymous shared in-memory DSN keeps the file inside one process
	// and visible across all *sql.DB connections opened from this DSN.
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func TestRuntimeRunsAndDrainsNoopTask(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	rt, err := jobs.New(jobs.Config{
		DB:              db,
		Workers:         2,
		ReleaseAfter:    time.Second,
		CleanupInterval: time.Hour,
		ShutdownTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}

	var ran int32
	done := make(chan struct{}, 1)
	rt.Register(backlite.NewQueue[noopTask](func(_ context.Context, task noopTask) error {
		atomic.AddInt32(&ran, 1)
		if task.Marker == "ping" {
			select {
			case done <- struct{}{}:
			default:
			}
		}
		return nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.Start(ctx)

	if _, err := rt.Client().Add(noopTask{Marker: "ping"}).Save(); err != nil {
		t.Fatalf("add: %v", err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("task did not run within deadline (ran=%d)", atomic.LoadInt32(&ran))
	}

	rt.Shutdown(context.Background())
	if got := atomic.LoadInt32(&ran); got < 1 {
		t.Errorf("expected at least one run, got %d", got)
	}
}

func TestRuntimeRecoversFromPanic(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	rt, err := jobs.New(jobs.Config{
		DB:              db,
		Workers:         1,
		ReleaseAfter:    time.Second,
		CleanupInterval: time.Hour,
		ShutdownTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("jobs.New: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	rt.Register(backlite.NewQueue[panicTask](func(context.Context, panicTask) error {
		defer wg.Done()
		panic("boom — dispatcher should swallow this")
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.Start(ctx)

	if _, err := rt.Client().Add(panicTask{}).Save(); err != nil {
		t.Fatalf("add: %v", err)
	}

	waitCh := make(chan struct{})
	go func() { wg.Wait(); close(waitCh) }()
	select {
	case <-waitCh:
	case <-time.After(5 * time.Second):
		t.Fatalf("panic handler did not run within deadline")
	}

	// If recovery worked, Shutdown returns cleanly within the timeout.
	rt.Shutdown(context.Background())
}
