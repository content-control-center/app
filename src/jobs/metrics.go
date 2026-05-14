package jobs

import (
	"expvar"
	"sync"
	"time"
)

// Metrics surfaces Ogen's job-queue health to operators (CON-69 §13).
// Counters are exposed via expvar at /debug/vars; latency averages
// live as a small in-process histogram so we don't pull a metrics
// library for the MVP. Backlite's UI already covers per-queue depth
// (upcoming/running/succeeded/failed) — this is what's left.
//
// Field naming is "ogen_jobs_*" so the expvar surface stays distinct
// from runtime/expvar's stock keys (memstats, cmdline, etc.).
var (
	// Submit lifecycle.
	ZernioSubmitSucceeded = expvar.NewInt("ogen_jobs_zernio_submit_succeeded")
	ZernioSubmitFailed    = expvar.NewInt("ogen_jobs_zernio_submit_failed")
	ZernioSubmitRetried   = expvar.NewInt("ogen_jobs_zernio_submit_retried")

	// Poll lifecycle.
	ZernioPollSucceeded = expvar.NewInt("ogen_jobs_zernio_poll_succeeded")
	ZernioPollFailed    = expvar.NewInt("ogen_jobs_zernio_poll_failed")
	ZernioPollRetried   = expvar.NewInt("ogen_jobs_zernio_poll_retried")

	// Cancel lifecycle.
	ZernioCancelSucceeded       = expvar.NewInt("ogen_jobs_zernio_cancel_succeeded")
	ZernioCancelAlreadyPublished = expvar.NewInt("ogen_jobs_zernio_cancel_already_published")
	ZernioCancelFailed          = expvar.NewInt("ogen_jobs_zernio_cancel_failed")

	// Reconciliation.
	ReconciliationTimeouts = expvar.NewInt("ogen_jobs_reconciliation_timeouts")

	// Post Log lifecycle.
	PostLogTruncations = expvar.NewInt("ogen_jobs_postlog_truncations")
	PostLogCleaned     = expvar.NewInt("ogen_jobs_postlog_cleaned")

	// Zernio API latency (millisecond average over a sliding window).
	zernioLatency = newLatency()
)

func init() {
	expvar.Publish("ogen_jobs_zernio_latency_ms_avg", expvar.Func(func() any { return zernioLatency.Avg() }))
}

// ObserveZernioCall records a single API call duration so the
// expvar-exposed average updates. Safe to call from any goroutine.
func ObserveZernioCall(d time.Duration) {
	zernioLatency.Add(d)
}

type latency struct {
	mu   sync.Mutex
	buf  [256]time.Duration
	idx  int
	full bool
}

func newLatency() *latency { return &latency{} }

func (l *latency) Add(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buf[l.idx] = d
	l.idx = (l.idx + 1) % len(l.buf)
	if l.idx == 0 {
		l.full = true
	}
}

func (l *latency) Avg() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := l.idx
	if l.full {
		n = len(l.buf)
	}
	if n == 0 {
		return 0
	}
	var total time.Duration
	for i := 0; i < n; i++ {
		total += l.buf[i]
	}
	return (total / time.Duration(n)).Milliseconds()
}
