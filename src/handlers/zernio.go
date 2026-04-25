package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/content-control-center/app/src/integrations/zernio"
)

// ZernioHandler exposes the integration endpoints under
// /api/integrations/zernio. Phase 2 ships only the operational repair
// endpoint; connect-link, platforms, accounts, and health are layered
// in later phases.
type ZernioHandler struct {
	integ        *zernio.Integration
	bootstrapper *zernio.Bootstrapper
	auth         fiber.Handler
}

func NewZernioHandler(
	integ *zernio.Integration,
	bootstrapper *zernio.Bootstrapper,
	auth fiber.Handler,
) *ZernioHandler {
	return &ZernioHandler{integ: integ, bootstrapper: bootstrapper, auth: auth}
}

func (h *ZernioHandler) Register(app *fiber.App) {
	g := app.Group("/api/integrations/zernio", h.auth)
	g.Post("/profile/repair", h.RepairProfile)
}

// RepairProfile godoc
// @Summary      Repair Zernio profile
// @Description  Triggers a fresh Zernio profile bootstrap for operational
// @Description  recovery. Idempotent: a successful repair leaves the
// @Description  integration in the "ok" state.
// @Tags         zernio
// @Produce      json
// @Security     CookieAuth
// @Success      202  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      409  {object}  map[string]string  "integration_disabled"
// @Failure      500  {object}  map[string]string
// @Router       /api/integrations/zernio/profile/repair [post]
func (h *ZernioHandler) RepairProfile(c *fiber.Ctx) error {
	if !h.integ.Enabled() {
		return fiber.NewError(fiber.StatusConflict, "integration_disabled")
	}
	if err := h.bootstrapper.Run(c.Context()); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"state": string(h.integ.State()),
	})
}
