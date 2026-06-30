package server

import (
	"log"
	"sync"

	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/usage"
)

// usageMetrics is registered once per process: expvar.NewInt panics on a
// duplicate name, so repeated server.New calls (integration tests) must share
// one Metrics instance rather than re-register the ogen_usage_* counters.
var (
	usageMetricsOnce sync.Once
	usageMetricsInst *usage.Metrics
)

func sharedUsageMetrics() *usage.Metrics {
	usageMetricsOnce.Do(func() { usageMetricsInst = usage.NewMetrics() })
	return usageMetricsInst
}

// usageDeps bundles the wired CON-86 metering pieces. limits + defaults are
// always present (the tenant_usage_limits table lives in the control-plane
// DB, so limits read/write works even when analytics is off). events,
// recorder, and checker are nil when analytics is disabled — all nil-safe.
type usageDeps struct {
	recorder *usage.Recorder
	checker  *usage.Checker
	events   repository.UsageRepository // analytics pool; nil when disabled
	limits   repository.UsageLimitsRepository
	defaults usage.Defaults
}

// initUsage wires the metering layer. When analyticsDB is nil (ANALYTICS_DSN
// empty, or the analytics DB was unreachable at boot) the recorder/checker/
// events are left nil: flows then record nothing and never enforce — the
// graceful-disable path (FR10) — while the limits config surface stays usable.
// The caller drains the recorder on shutdown via recorder.Close, registered
// AFTER the river/zernio producers stop so loop() never exits while a worker is
// still calling Record() (see server.New).
func initUsage(cfg *config.Config, db, analyticsDB *bun.DB) usageDeps {
	deps := usageDeps{
		limits: repository.NewUsageLimitsRepository(db),
		defaults: usage.Defaults{
			DailyCapMicros:   cfg.UsageDefaultDailyCapMicros,
			MonthlyCapMicros: cfg.UsageDefaultMonthlyCapMicros,
			Mode:             cfg.UsageDefaultMode,
		},
	}

	if analyticsDB == nil {
		log.Println("usage analytics disabled (ANALYTICS_DSN empty)")
		return deps
	}

	metrics := sharedUsageMetrics()
	deps.events = repository.NewUsageRepository(analyticsDB)
	deps.recorder = usage.NewRecorder(deps.events, metrics, usage.Config{})
	deps.checker = usage.NewChecker(deps.limits, deps.events, deps.defaults, metrics, 0)

	log.Println("usage analytics enabled")
	return deps
}
