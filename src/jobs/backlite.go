// Package jobs hosts Ogen's background-job machinery (CON-69). All
// queues live as sub-packages or files in this package and are
// registered against a single Backlite client at boot.
//
// Backlite was chosen because it is SQLite-native (no new infra),
// type-safe via Go generics, transaction-aware so an enqueue can join
// a Post-status-change transaction, and ships with a lightweight UI
// for ops debugging.
package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/mikestefanello/backlite"
)

// Runtime is the handle the rest of the app holds onto for enqueuing
// tasks and shutting the worker pool down. It hides the concrete
// Backlite client so call sites don't have to depend on the package
// directly.
type Runtime struct {
	client          *backlite.Client
	shutdownTimeout time.Duration
}

// Config is the boot-time knob set. Sensible defaults live in
// src/config/config.go; this struct just transports them.
type Config struct {
	DB              *sql.DB
	Workers         int
	ReleaseAfter    time.Duration
	CleanupInterval time.Duration
	ShutdownTimeout time.Duration
}

// New constructs a Runtime, installs the backlite schema (idempotent),
// and returns it. The returned Runtime is not yet started — callers
// must Register their queues, then call Start.
func New(cfg Config) (*Runtime, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("jobs: DB is required")
	}
	if cfg.Workers < 1 {
		cfg.Workers = 4
	}
	if cfg.ReleaseAfter <= 0 {
		cfg.ReleaseAfter = 5 * time.Minute
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = time.Hour
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 30 * time.Second
	}

	client, err := backlite.NewClient(backlite.ClientConfig{
		DB:              cfg.DB,
		NumWorkers:      cfg.Workers,
		ReleaseAfter:    cfg.ReleaseAfter,
		CleanupInterval: cfg.CleanupInterval,
		Logger:          stdLogger{},
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: backlite client: %w", err)
	}
	if err := client.Install(); err != nil {
		return nil, fmt.Errorf("jobs: backlite install: %w", err)
	}
	return &Runtime{client: client, shutdownTimeout: cfg.ShutdownTimeout}, nil
}

// Client exposes the underlying Backlite client so packages defining
// queues can register them.
func (r *Runtime) Client() *backlite.Client { return r.client }

// Register registers a queue with the Backlite client. Wraps
// client.Register so call sites don't import backlite directly.
func (r *Runtime) Register(q backlite.Queue) {
	r.client.Register(q)
}

// Start spins up the worker pool. It returns immediately; the pool
// runs until Shutdown is called.
func (r *Runtime) Start(ctx context.Context) {
	r.client.Start(ctx)
	log.Printf("jobs: backlite worker pool started")
}

// Shutdown drains the worker pool, waiting up to shutdownTimeout for
// in-flight tasks to finish. Logs whether the wait succeeded.
func (r *Runtime) Shutdown(ctx context.Context) {
	deadline, cancel := context.WithTimeout(ctx, r.shutdownTimeout)
	defer cancel()
	if r.client.Stop(deadline) {
		log.Printf("jobs: backlite worker pool stopped cleanly")
		return
	}
	log.Printf("jobs: backlite shutdown deadline (%s) exceeded; some tasks may not have completed", r.shutdownTimeout)
}

// stdLogger adapts the standard log package to backlite.Logger so we
// keep one log surface across the app.
type stdLogger struct{}

func (stdLogger) Info(msg string, args ...any)  { log.Printf("backlite: "+msg, args...) }
func (stdLogger) Error(msg string, args ...any) { log.Printf("backlite ERROR: "+msg, args...) }
