package server

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
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

// initUsage wires the CON-86 metering layer onto the analytics pool. When
// analyticsDB is nil (ANALYTICS_DSN empty, or the analytics DB was unreachable
// at boot) it returns (nil, nil): the recorder and checker are nil-safe, so
// flows then record nothing and never enforce — the graceful-disable path
// (FR10). The recorder's background writer is drained on shutdown.
func initUsage(app *fiber.App, cfg *config.Config, db, analyticsDB *bun.DB) (*usage.Recorder, *usage.Checker) {
	if analyticsDB == nil {
		log.Println("usage analytics disabled (ANALYTICS_DSN empty)")
		return nil, nil
	}

	metrics := sharedUsageMetrics()
	usageRepo := repository.NewUsageRepository(analyticsDB) // analytics pool
	limitsRepo := repository.NewUsageLimitsRepository(db)   // control-plane pool

	recorder := usage.NewRecorder(usageRepo, metrics, usage.Config{})
	checker := usage.NewChecker(limitsRepo, usageRepo, usage.Defaults{
		DailyCapMicros:   cfg.UsageDefaultDailyCapMicros,
		MonthlyCapMicros: cfg.UsageDefaultMonthlyCapMicros,
		Mode:             cfg.UsageDefaultMode,
	}, metrics, 0)

	app.Hooks().OnShutdown(func() error {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return recorder.Close(sctx)
	})

	log.Println("usage analytics enabled")
	return recorder, checker
}
