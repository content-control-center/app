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
	"os"
	"path/filepath"

	"github.com/uptrace/bun"

	_ "github.com/ogen-app/ogen/docs"
	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/database"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/secrets"
	"github.com/ogen-app/ogen/src/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := database.New(cfg.DSN, cfg.Debug)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	db.DB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.DB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	defer db.Close()

	if err := database.Migrate(context.Background(), db); err != nil {
		log.Fatalf("run migrations: %v", err)
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
			log.Printf("usage analytics: connect failed, disabling (fail-open): %v", aerr)
		default:
			if merr := database.MigrateAnalytics(context.Background(), adb); merr != nil {
				log.Printf("usage analytics: migrate failed, disabling (fail-open): %v", merr)
				_ = adb.Close()
			} else {
				analyticsDB = adb
				defer analyticsDB.Close()
			}
		}
	}

	// Envelope encryption: load (or generate) the KEK, build a
	// Cipher, then expose Get/Set through SecretStore. Boot fails on
	// any KEK file error — running without an unwrapper is worse than
	// not booting because rotated keys would silently be unrecoverable.
	cipher, kekSrc, err := secrets.InitCipher(cfg.KEKPath)
	if err != nil {
		log.Fatalf("init secret cipher: %v", err)
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
		log.Fatalf("migrate secrets from env: %v", err)
	}
	secrets.LogBootSummary(kekSrc, filepath.Join(cfg.KEKPath, secrets.KEKFilename), bootResult)

	app, err := server.New(context.Background(), db, analyticsDB, cfg, store)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	log.Printf("listening on %s", cfg.Addr)
	if err := app.Listen(cfg.Addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
