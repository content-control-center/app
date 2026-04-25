package server

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/integrations/zernio"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

// initZernio constructs the Zernio integration, the Bootstrapper, and a
// SettingsStore adapter wrapping the host's SettingRepository. Ping and
// the initial profile bootstrap run in a background goroutine — Ogen
// boot never blocks on Zernio reachability.
//
// State transitions, all owned by the spawned goroutine:
//
//   - No API key            → log WARN once, integration stays in
//                             StateDisabled; nil Bootstrapper returned.
//   - Ping returns 401      → log ERROR, StateDisabled; admin must
//                             repair via /profile/repair.
//   - Ping transport / 5xx  → log WARN, StateDegraded; bootstrap is
//                             skipped this boot, retried by the worker
//                             (Phase 5) or admin repair.
//   - Ping returns 200      → run Bootstrapper.Run; on success
//                             StateOK, otherwise StateDegraded.
func initZernio(
	ctx context.Context,
	cfg *config.Config,
	settingRepo repository.SettingRepository,
) (*zernio.Integration, *zernio.Bootstrapper) {
	if cfg.ZernioAPIKey == "" {
		log.Printf("zernio: integration disabled — ZERNIO_API_KEY not set")
		return zernio.NewIntegration(nil), nil
	}

	client := zernio.NewClient(cfg.ZernioAPIKey, cfg.ZernioBaseURL, cfg.ZernioHTTPTimeout)
	integ := zernio.NewIntegration(client)
	store := &settingsStoreAdapter{repo: settingRepo}
	bootstrapper := zernio.NewBootstrapper(integ, store)

	go warmupZernio(ctx, integ, bootstrapper)
	return integ, bootstrapper
}

// warmupZernio validates the API key with a single Ping then runs the
// profile bootstrap. Errors are logged; the goroutine exits cleanly on
// ctx cancellation (process shutdown).
func warmupZernio(ctx context.Context, integ *zernio.Integration, bootstrapper *zernio.Bootstrapper) {
	if err := integ.Client.Ping(ctx); err != nil {
		if zernio.IsStatus(err, http.StatusUnauthorized) {
			log.Printf("zernio: API key rejected (401) — integration disabled until repaired")
			integ.SetState(zernio.StateDisabled)
			return
		}
		var apiErr *zernio.APIError
		if errors.As(err, &apiErr) {
			log.Printf("zernio: ping failed with HTTP %d — staying degraded", apiErr.Status)
		} else {
			log.Printf("zernio: ping failed (%v) — staying degraded", err)
		}
		integ.SetState(zernio.StateDegraded)
		return
	}

	log.Printf("zernio: API key validated — base=%s", integ.Client.BaseURL())
	integ.SetState(zernio.StateDegraded) // promoted to StateOK by Bootstrapper.Run on success

	if err := bootstrapper.Run(ctx); err != nil {
		log.Printf("zernio: initial bootstrap: %v", err)
	}
}

// settingsStoreAdapter bridges repository.SettingRepository to the
// narrow SettingsStore contract the zernio package consumes.
type settingsStoreAdapter struct {
	repo repository.SettingRepository
}

func (a *settingsStoreAdapter) Get(ctx context.Context, key string) (string, bool, error) {
	s, err := a.repo.GetByKey(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return s.Value, true, nil
}

func (a *settingsStoreAdapter) Set(ctx context.Context, key, value string) error {
	return a.repo.Upsert(ctx, &models.Setting{Key: key, Value: value})
}

func (a *settingsStoreAdapter) Delete(ctx context.Context, key string) error {
	_, err := a.repo.Delete(ctx, key)
	return err
}
