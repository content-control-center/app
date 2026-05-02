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
	"path/filepath"

	_ "github.com/content-control-center/app/docs"
	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/database"
	"github.com/content-control-center/app/src/repository"
	"github.com/content-control-center/app/src/secrets"
	"github.com/content-control-center/app/src/server"
	webstatic "github.com/content-control-center/app/web"
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
	defer db.Close()

	if err := database.Migrate(context.Background(), db); err != nil {
		log.Fatalf("run migrations: %v", err)
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
	})
	if err != nil {
		log.Fatalf("migrate secrets from env: %v", err)
	}
	secrets.LogBootSummary(kekSrc, filepath.Join(cfg.KEKPath, secrets.KEKFilename), bootResult)

	staticFS, err := webstatic.DistFS()
	if err != nil {
		log.Fatalf("load static assets: %v", err)
	}

	app, err := server.New(context.Background(), db, staticFS, cfg, store)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}

	log.Printf("listening on %s", cfg.Addr)
	if err := app.Listen(cfg.Addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
