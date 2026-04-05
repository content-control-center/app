// @title           Content Control Center API
// @version         1.0
// @description     REST API for the Content Control Center application.
// @host            localhost:3000
// @BasePath        /
package main

import (
	"context"
	"log"

	_ "github.com/content-control-center/app/docs"
	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/database"
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

	staticFS, err := webstatic.DistFS()
	if err != nil {
		log.Fatalf("load static assets: %v", err)
	}

	app := server.New(db, staticFS)

	log.Printf("listening on %s", cfg.Addr)
	if err := app.Listen(cfg.Addr); err != nil {
		log.Fatalf("server: %v", err)
	}
}
