package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/uptrace/bun"
)

type HealthHandler struct {
	db *bun.DB
}

func NewHealthHandler(db *bun.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Register(app *fiber.App) {
	app.Get("/api/health", h.Health)
}

// Health godoc
// @Summary Health check
// @Success 200 {object} map[string]string
func (h *HealthHandler) Health(c *fiber.Ctx) error {
	if err := h.db.PingContext(c.Context()); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "unhealthy",
			"error":  err.Error(),
		})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}
