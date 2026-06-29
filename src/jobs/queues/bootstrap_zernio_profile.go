package queues

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/riverqueue/river"

	"github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// BootstrapZernioProfileQueue eagerly provisions a tenant's Zernio profile at
// registration time (CON-102 §6.1). Signup enqueues one task per new tenant
// inside its transaction (transactional outbox), so the profile is created in
// the background without ever blocking the signup request on Zernio. The lazy
// on-connect bootstrap (handlers/zernio.go) remains the guaranteed fallback, so
// this job is best-effort: when Zernio is unreachable or unconfigured it gives
// up cleanly rather than holding the tenant hostage.
const BootstrapZernioProfileQueue = "bootstrap_zernio_profile"

// BootstrapZernioProfileTask carries the tenant whose profile to provision.
type BootstrapZernioProfileTask struct {
	TenantID string `json:"tenant_id"`
}

// Kind implements river.JobArgs.
func (BootstrapZernioProfileTask) Kind() string { return BootstrapZernioProfileQueue }

// InsertOpts bounds retries. No UniqueOpts is needed: signup enqueues exactly
// one task per tenant (tenant ids are unique and the insert commits with the
// tenant row), and Bootstrapper.Run is adopt-or-create idempotent, so a retry —
// or a race with the lazy on-connect path — converges on a single profile id.
func (BootstrapZernioProfileTask) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 5}
}

// BootstrapZernioProfileProcessor runs one eager-provisioning attempt. It holds
// the same Bootstrapper + Integration the connect-link handler uses; the
// Bootstrapper already serialises concurrent Runs and writes the profile
// settings under the task's tenant scope.
type BootstrapZernioProfileProcessor struct {
	river.WorkerDefaults[BootstrapZernioProfileTask]
	Integration  *zernio.Integration
	Bootstrapper *zernio.Bootstrapper
}

// Work is the River entrypoint. Unlike the cross-tenant background sweeps, this
// job is scoped to a single tenant, so it runs under that tenant's context (the
// Bootstrapper reads/writes tenant-scoped settings and names the profile after
// the context's tenant).
func (p *BootstrapZernioProfileProcessor) Work(ctx context.Context, job *river.Job[BootstrapZernioProfileTask]) error {
	tid := job.Args.TenantID
	if tid == "" {
		log.Printf("zernio: eager profile bootstrap skipped — empty tenant_id")
		return nil
	}
	if p.Bootstrapper == nil || p.Integration == nil {
		log.Printf("zernio: eager profile bootstrap skipped — integration not wired (tenant=%s)", tid)
		return nil
	}

	ctx = tenantctx.With(ctx, tid)

	// No key / permanently disabled: the lazy on-connect path is the guaranteed
	// fallback, so don't burn retries waiting for a key that may never arrive.
	if !p.Integration.Enabled() {
		log.Printf("zernio: eager profile bootstrap skipped — integration disabled (tenant=%s); lazy path will provision on first connect", tid)
		return nil
	}

	if err := p.Bootstrapper.Run(ctx); err != nil {
		// A rejected key (401) flips the integration to disabled; retrying in a
		// tight loop won't recover it, and the lazy path still covers first
		// connect — so give up cleanly.
		if p.Integration.State() == zernio.StateDisabled {
			log.Printf("zernio: eager profile bootstrap gave up — integration disabled (tenant=%s): %v", tid, err)
			return nil
		}
		// Transient / degraded (network, 5xx, 429): let River retry with backoff.
		return fmt.Errorf("zernio: eager profile bootstrap (tenant=%s): %w", tid, err)
	}
	return nil
}

// Timeout is the per-attempt context deadline. A single Run is a small handful
// of Zernio calls; 30s comfortably covers the retry backoff inside Run.
func (p *BootstrapZernioProfileProcessor) Timeout(*river.Job[BootstrapZernioProfileTask]) time.Duration {
	return 30 * time.Second
}

func init() {
	register(func(w *river.Workers, d Deps) {
		river.AddWorker(w, &BootstrapZernioProfileProcessor{
			Integration:  d.Integration,
			Bootstrapper: d.ProfileBootstrapper,
		})
	})
}
