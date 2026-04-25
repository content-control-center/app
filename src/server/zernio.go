package server

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/integrations/zernio"
)

// initZernio constructs the Zernio integration controller and validates
// the configured API key against Zernio. It never aborts boot — the
// integration starts disabled or degraded on failure and the rest of
// Ogen runs normally.
//
// State transitions performed here:
//
//   - No API key            → log WARN, return integration with
//                             Client == nil and StateDisabled.
//   - Ping returns 401      → log ERROR, StateDisabled (admin must
//                             repair via /profile/repair).
//   - Ping transport / 5xx  → log WARN, StateDegraded (worker /
//                             bootstrap retry on next opportunity).
//   - Ping returns 200      → StateDegraded (profile bootstrap in
//                             Phase 2 promotes to StateOK).
func initZernio(ctx context.Context, cfg *config.Config) *zernio.Integration {
	if cfg.ZernioAPIKey == "" {
		log.Printf("zernio: integration disabled — ZERNIO_API_KEY not set")
		return zernio.NewIntegration(nil)
	}

	client := zernio.NewClient(cfg.ZernioAPIKey, cfg.ZernioBaseURL, cfg.ZernioHTTPTimeout)
	integ := zernio.NewIntegration(client)

	if err := client.Ping(ctx); err != nil {
		if zernio.IsStatus(err, http.StatusUnauthorized) {
			log.Printf("zernio: API key rejected (401) — integration disabled until repaired")
			return integ
		}
		var apiErr *zernio.APIError
		if errors.As(err, &apiErr) {
			log.Printf("zernio: ping failed with HTTP %d — starting in degraded mode", apiErr.Status)
		} else {
			log.Printf("zernio: ping failed (%v) — starting in degraded mode", err)
		}
		integ.SetState(zernio.StateDegraded)
		return integ
	}

	log.Printf("zernio: API key validated — base=%s", client.BaseURL())
	integ.SetState(zernio.StateDegraded) // promoted to StateOK after profile bootstrap (Phase 2)
	return integ
}
