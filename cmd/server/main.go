package main

import (
	"context"
	"log"

	"github.com/content-control-center/app/internal/config"
	"github.com/content-control-center/app/internal/database"
	"github.com/content-control-center/app/internal/server"
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
