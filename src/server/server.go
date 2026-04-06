package server

import (
	"errors"
	"io/fs"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/handlers"
)

func New(db *bun.DB, staticFS fs.FS, cfg *config.Config) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: defaultErrorHandler,
	})

	app.Use(recover.New())
	app.Use(logger.New())

	// API routes
	auth := handlers.RequireAuth(db, cfg.SessionCookieName)
	handlers.NewHealthHandler(db).Register(app)
	handlers.NewUsersHandler(db, auth).Register(app)
	handlers.NewSessionsHandler(db, cfg.SessionCookieName).Register(app)

	// Serve the embedded React SPA for all non-API routes.
	app.Use("/", filesystem.New(filesystem.Config{
		Root:         http.FS(staticFS),
		Index:        "index.html",
		NotFoundFile: "index.html", // enables client-side routing
		Browse:       false,
	}))

	return app
}

func defaultErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := errors.AsType[*fiber.Error](err); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}
