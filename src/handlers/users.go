package handlers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

type UsersHandler struct {
	db   *bun.DB
	auth fiber.Handler
}

func NewUsersHandler(db *bun.DB, auth fiber.Handler) *UsersHandler {
	return &UsersHandler{db: db, auth: auth}
}

func (h *UsersHandler) Register(app *fiber.App) {
	g := app.Group("/api/users")
	g.Post("/", h.Create)                     // public — registration
	g.Get("/", h.auth, h.List)                // protected
	g.Get("/:id", h.auth, h.Get)              // protected
	g.Put("/:id", h.auth, h.Update)           // protected
	g.Delete("/:id", h.auth, h.Delete)        // protected
}

type createUserRequest struct {
	Name     string `json:"name"     validate:"required"`
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type updateUserRequest struct {
	Name     string `json:"name"     validate:"required"`
	Email    string `json:"email"    validate:"required,email"`
	// Password is optional on update; when provided it must be at least 8 characters.
	Password string `json:"password" validate:"omitempty,min=8"`
}

// List godoc
// @Summary      List users
// @Description  Returns all users ordered by creation date.
// @Tags         users
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   models.User
// @Failure      401  {object}  map[string]string
// @Router       /api/users [get]
func (h *UsersHandler) List(c *fiber.Ctx) error {
	var users []models.User
	if err := h.db.NewSelect().Model(&users).OrderExpr("created_at ASC").Scan(c.Context()); err != nil {
		return err
	}
	return c.JSON(users)
}

// Create godoc
// @Summary      Create user
// @Description  Creates a new user and returns it with a generated Sqid. Does not require authentication.
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        body  body      createUserRequest  true  "User payload"
// @Success      201   {object}  models.User
// @Failure      400   {object}  map[string]string
// @Router       /api/users [post]
func (h *UsersHandler) Create(c *fiber.Ctx) error {
	var req createUserRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	id, err := models.NewID()
	if err != nil {
		return err
	}

	hash, err := models.HashPassword(req.Password)
	if err != nil {
		return err
	}

	user := &models.User{
		ID:           id,
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: hash,
	}
	if _, err := h.db.NewInsert().Model(user).Exec(c.Context()); err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(user)
}

// Get godoc
// @Summary      Get user
// @Description  Returns a single user by Sqid.
// @Tags         users
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "User Sqid"
// @Success      200  {object}  models.User
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/users/{id} [get]
func (h *UsersHandler) Get(c *fiber.Ctx) error {
	user := new(models.User)
	err := h.db.NewSelect().Model(user).Where("u.id = ?", c.Params("id")).Scan(c.Context())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "user not found")
		}
		return err
	}
	return c.JSON(user)
}

// Update godoc
// @Summary      Update user
// @Description  Updates name and/or email of an existing user.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string             true  "User Sqid"
// @Param        body  body      updateUserRequest  true  "User payload"
// @Success      200   {object}  models.User
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/users/{id} [put]
func (h *UsersHandler) Update(c *fiber.Ctx) error {
	var req updateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	user := new(models.User)
	err := h.db.NewSelect().Model(user).Where("u.id = ?", c.Params("id")).Scan(c.Context())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "user not found")
		}
		return err
	}

	user.Name = req.Name
	user.Email = req.Email
	user.UpdatedAt = time.Now().UTC()

	if req.Password != "" {
		hash, err := models.HashPassword(req.Password)
		if err != nil {
			return err
		}
		user.PasswordHash = hash
	}

	if _, err := h.db.NewUpdate().Model(user).WherePK().Exec(c.Context()); err != nil {
		return err
	}

	return c.JSON(user)
}

// Delete godoc
// @Summary      Delete user
// @Description  Deletes a user by Sqid.
// @Tags         users
// @Security     CookieAuth
// @Param        id   path  string  true  "User Sqid"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/users/{id} [delete]
func (h *UsersHandler) Delete(c *fiber.Ctx) error {
	res, err := h.db.NewDelete().Model((*models.User)(nil)).Where("id = ?", c.Params("id")).Exec(c.Context())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
