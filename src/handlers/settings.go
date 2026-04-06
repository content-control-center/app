package handlers

import (
	"database/sql"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

type SettingsHandler struct {
	db   *bun.DB
	auth fiber.Handler
}

func NewSettingsHandler(db *bun.DB, auth fiber.Handler) *SettingsHandler {
	return &SettingsHandler{db: db, auth: auth}
}

func (h *SettingsHandler) Register(app *fiber.App) {
	g := app.Group("/api/settings")
	g.Get("/", h.auth, h.List)
	g.Get("/:key", h.auth, h.Get)
	g.Put("/:key", h.auth, h.Upsert)
	g.Delete("/:key", h.auth, h.Delete)
}

type upsertSettingRequest struct {
	Value string `json:"value" validate:"required"`
}

// List godoc
// @Summary      List settings
// @Description  Returns all key-value settings.
// @Tags         settings
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   models.Setting
// @Failure      401  {object}  map[string]string
// @Router       /api/settings [get]
func (h *SettingsHandler) List(c *fiber.Ctx) error {
	var settings []models.Setting
	if err := h.db.NewSelect().Model(&settings).OrderExpr("key ASC").Scan(c.Context()); err != nil {
		return err
	}
	return c.JSON(settings)
}

// Get godoc
// @Summary      Get setting
// @Description  Returns a single setting by key.
// @Tags         settings
// @Produce      json
// @Security     CookieAuth
// @Param        key  path      string  true  "Setting key"
// @Success      200  {object}  models.Setting
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/settings/{key} [get]
func (h *SettingsHandler) Get(c *fiber.Ctx) error {
	setting := new(models.Setting)
	err := h.db.NewSelect().Model(setting).Where("st.key = ?", c.Params("key")).Scan(c.Context())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "setting not found")
		}
		return err
	}
	return c.JSON(setting)
}

// Upsert godoc
// @Summary      Upsert setting
// @Description  Creates or updates a setting by key.
// @Tags         settings
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        key   path      string                true  "Setting key"
// @Param        body  body      upsertSettingRequest  true  "Setting value"
// @Success      200   {object}  models.Setting
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Router       /api/settings/{key} [put]
func (h *SettingsHandler) Upsert(c *fiber.Ctx) error {
	var req upsertSettingRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	setting := &models.Setting{
		Key:   c.Params("key"),
		Value: req.Value,
	}
	if _, err := h.db.NewInsert().Model(setting).
		On("CONFLICT (key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(c.Context()); err != nil {
		return err
	}

	return c.JSON(setting)
}

// Delete godoc
// @Summary      Delete setting
// @Description  Deletes a setting by key.
// @Tags         settings
// @Security     CookieAuth
// @Param        key  path  string  true  "Setting key"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/settings/{key} [delete]
func (h *SettingsHandler) Delete(c *fiber.Ctx) error {
	res, err := h.db.NewDelete().Model((*models.Setting)(nil)).Where("key = ?", c.Params("key")).Exec(c.Context())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fiber.NewError(fiber.StatusNotFound, "setting not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
