package handlers

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

type UsersHandler struct {
	repo        repository.UserRepository
	settingRepo repository.SettingRepository
	auth        fiber.Handler
}

func NewUsersHandler(repo repository.UserRepository, settingRepo repository.SettingRepository, auth fiber.Handler) *UsersHandler {
	return &UsersHandler{repo: repo, settingRepo: settingRepo, auth: auth}
}

func (h *UsersHandler) Register(app *fiber.App) {
	app.Get("/api/current_user", h.auth, h.CurrentUser) // always protected

	g := app.Group("/api/users")
	g.Post("/", h.conditionalAuth, h.Create)  // open while setup_complete=false, protected after
	g.Get("/", h.auth, h.List)                // always protected
	g.Get("/:id", h.auth, h.Get)              // always protected
	g.Put("/:id", h.auth, h.Update)           // always protected
	g.Delete("/:id", h.auth, h.Delete)        // always protected
}

// conditionalAuth requires authentication only after setup is complete.
// This allows the first user to be created without a session during initial setup.
func (h *UsersHandler) conditionalAuth(c *fiber.Ctx) error {
	complete, err := setupComplete(c.Context(), h.settingRepo)
	if err != nil {
		return err
	}
	if complete {
		return h.auth(c)
	}
	return c.Next()
}

// requireSelf returns 403 unless the authenticated session belongs to the user
// identified by the :id route parameter. The auth middleware must have already
// stored the session in c.Locals("session").
func requireSelf(c *fiber.Ctx) error {
	session, ok := c.Locals("session").(*models.Session)
	if !ok || session == nil {
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}
	if session.UserID != c.Params("id") {
		return fiber.NewError(fiber.StatusForbidden, "forbidden")
	}
	return nil
}

// setupComplete returns true when the "setup_complete" setting is "true".
func setupComplete(ctx context.Context, repo repository.SettingRepository) (bool, error) {
	setting, err := repo.GetByKey(ctx, "setup_complete")
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return setting.Value == "true", nil
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

// CurrentUser godoc
// @Summary      Get current user
// @Description  Returns the authenticated user derived from the active session cookie.
// @Tags         users
// @Produce      json
// @Security     CookieAuth
// @Success      200  {object}  models.User
// @Failure      401  {object}  map[string]string
// @Router       /api/current_user [get]
func (h *UsersHandler) CurrentUser(c *fiber.Ctx) error {
	session := c.Locals("session").(*models.Session)
	user, err := h.repo.GetByID(c.Context(), session.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusUnauthorized, "user not found")
		}
		return err
	}
	return c.JSON(user)
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
	users, err := h.repo.List(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(users)
}

// Create godoc
// @Summary      Create user
// @Description  Creates a new user. Open (no auth required) while setup_complete=false; requires authentication once setup is complete.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      createUserRequest  true  "User payload"
// @Success      201   {object}  models.User
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
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
	if err := h.repo.Create(c.Context(), user); err != nil {
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
	user, err := h.repo.GetByID(c.Context(), c.Params("id"))
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
	if err := requireSelf(c); err != nil {
		return err
	}

	var req updateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	user, err := h.repo.GetByID(c.Context(), c.Params("id"))
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

	if err := h.repo.Update(c.Context(), user); err != nil {
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
	if err := requireSelf(c); err != nil {
		return err
	}

	deleted, err := h.repo.Delete(c.Context(), c.Params("id"))
	if err != nil {
		return err
	}
	if !deleted {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
