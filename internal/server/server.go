package server

import (
	"io/fs"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	fiberSwagger "github.com/swaggo/fiber-swagger"
	"github.com/uptrace/bun"

	"github.com/content-control-center/app/internal/handlers"
)

func New(db *bun.DB, staticFS fs.FS) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: defaultErrorHandler,
	})

	app.Use(recover.New())
	app.Use(logger.New())

	// API routes
	handlers.NewHealthHandler(db).Register(app)

	// Swagger UI — available at /swagger/index.html
	app.Get("/swagger/*", fiberSwagger.WrapHandler)

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
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}
