// @title           Content Control Center API
// @version         1.0
// @description     REST API for the Content Control Center application.
// @host            localhost:9001
// @BasePath        /
//
// @securityDefinitions.apikey  CookieAuth
// @in                          cookie
// @name                        c3_session
// @description                 Session token obtained from POST /api/sessions (login).
package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/uptrace/bun"

	_ "github.com/ogen-app/ogen/docs"
	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/database"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/secrets"
	"github.com/ogen-app/ogen/src/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// The logger needs cfg to build, so this one boot error necessarily
		// predates it — fail fast on the stdlib logger.
		log.Fatalf("load config: %v", err)
	}

	// Install the structured logger before anything else logs, so even early
	// boot errors are structured and any stray stdlib log.Print is bridged.
	logging.New(cfg)

	db, err := database.New(cfg.DSN, cfg.Debug)
	if err != nil {
		fatal("connect to database", err)
	}
	db.DB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.DB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	defer db.Close()

	if err := database.Migrate(context.Background(), db); err != nil {
		fatal("run migrations", err)
	}

	// CON-86: the isolated analytics (TimescaleDB) pool for usage_events. When
	// ANALYTICS_DSN is unset, analytics is disabled. A connect/migrate failure
	// at boot is NOT fatal — usage is analytics-grade and must never take down
	// the API (fail-open, FR10) — so we log and proceed with it disabled.
	var analyticsDB *bun.DB
	if cfg.AnalyticsDSN != "" {
		adb, aerr := database.NewAnalytics(cfg.AnalyticsDSN, cfg.Debug)
		switch {
		case aerr != nil:
			slog.Warn("usage analytics connect failed, disabling (fail-open)",
				logging.AttrComponent, "boot", logging.AttrError, aerr)
		default:
			if merr := database.MigrateAnalytics(context.Background(), adb); merr != nil {
				slog.Warn("usage analytics migrate failed, disabling (fail-open)",
					logging.AttrComponent, "boot", logging.AttrError, merr)
				_ = adb.Close()
			} else {
				analyticsDB = adb
				defer analyticsDB.Close()
				// CON-125 Track B: one-time, idempotent backfill of the legacy
				// main-DB post_analytics snapshots into the analytics DB. Best-
				// effort — a failure must not take down boot (the legacy table
				// is retained until a separate later migration drops it).
				if n, berr := repository.BackfillPostAnalytics(context.Background(), db, adb); berr != nil {
					slog.Warn("post_analytics backfill failed (non-fatal)",
						logging.AttrComponent, "boot", logging.AttrError, berr)
				} else if n > 0 {
					slog.Info("post_analytics backfilled",
						logging.AttrComponent, "boot", "rows", n)
				}
				// CON-125: one-time, idempotent migration of the historical
				// post_logs audit trail into activity_events (curated to the
				// activity taxonomy). Best-effort — never fatal to boot.
				if n, berr := repository.BackfillPostLogsToActivity(context.Background(), db, adb); berr != nil {
					slog.Warn("post_logs → activity_events backfill failed (non-fatal)",
						logging.AttrComponent, "boot", logging.AttrError, berr)
				} else if n > 0 {
					slog.Info("post_logs migrated to activity_events",
						logging.AttrComponent, "boot", "rows", n)
				}
			}
		}
	}

	// Envelope encryption: load (or generate) the KEK, build a
	// Cipher, then expose Get/Set through SecretStore. Boot fails on
	// any KEK file error — running without an unwrapper is worse than
	// not booting because rotated keys would silently be unrecoverable.
	cipher, kekSrc, err := secrets.InitCipher(cfg.KEKPath)
	if err != nil {
		fatal("init secret cipher", err)
	}
	store := secrets.NewStore(repository.NewSecretRepository(db), cipher)

	bootResult, err := secrets.MigrateFromEnv(context.Background(), store, []secrets.EnvSource{
		{Name: secrets.NameAnthropicAPIKey, EnvValue: cfg.AnthropicAPIKey},
		{Name: secrets.NameZernioAPIKey, EnvValue: cfg.ZernioAPIKey},
		// GEMINI_API_KEY is read straight from the env (it is not a typed Config
		// field) — first-boot seed only; thereafter set/rotated via the secrets
		// API (CON-104).
		{Name: secrets.NameGeminiAPIKey, EnvValue: os.Getenv("GEMINI_API_KEY")},
	})
	if err != nil {
		fatal("migrate secrets from env", err)
	}
	secrets.LogBootSummary(kekSrc, filepath.Join(cfg.KEKPath, secrets.KEKFilename), bootResult)

	app, err := server.New(context.Background(), db, analyticsDB, cfg, store)
	if err != nil {
		fatal("init server", err)
	}

	slog.Info("server listening", logging.AttrComponent, "boot", "addr", cfg.Addr)
	if err := app.Listen(cfg.Addr); err != nil {
		fatal("server exited", err)
	}
}

// fatal logs an unrecoverable boot error at ERROR level and exits non-zero.
// slog has no Fatal; this is its idiomatic replacement and, like log.Fatal, it
// intentionally skips deferred cleanup — acceptable for a boot failure.
func fatal(msg string, err error) {
	slog.Error(msg, logging.AttrComponent, "boot", logging.AttrError, err)
	os.Exit(1)
}
