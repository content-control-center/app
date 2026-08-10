package handlers

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/activity"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

type UsersHandler struct {
	// db backs the password-change path, which must update the credential and
	// revoke the user's other sessions in one transaction (CON-193). Every other
	// operation goes through repo.
	db   *bun.DB
	repo repository.UserRepository
	// settingRepo backed the setup_complete bootstrap gate, removed in CON-97
	// (signup via POST /api/tenants is the sole bootstrap). Retained on the
	// constructor to avoid churn across the ~37 call sites; revisit when
	// settings become per-tenant (PR3).
	settingRepo repository.SettingRepository
	auth        fiber.Handler
	// activity records CON-125 authentication-category events (user_created,
	// user_updated, user_deleted). nil is a no-op. Wired via SetActivityRecorder.
	activity *activity.Recorder
}

func NewUsersHandler(db *bun.DB, repo repository.UserRepository, settingRepo repository.SettingRepository, auth fiber.Handler) *UsersHandler {
	return &UsersHandler{db: db, repo: repo, settingRepo: settingRepo, auth: auth}
}

// SetActivityRecorder wires the CON-125 activity recorder (nil-safe no-op).
func (h *UsersHandler) SetActivityRecorder(r *activity.Recorder) { h.activity = r }

func (h *UsersHandler) Register(app *fiber.App) {
	app.Get("/api/current_user", h.auth, h.CurrentUser) // always protected

	g := app.Group("/api/users")
	g.Post("/", h.auth, h.Create)      // new users join the caller's tenant (CON-97)
	g.Get("/", h.auth, h.List)         // always protected
	g.Get("/:id", h.auth, h.Get)       // always protected
	g.Put("/:id", h.auth, h.Update)    // always protected
	g.Delete("/:id", h.auth, h.Delete) // always protected
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

type createUserRequest struct {
	Name     string `json:"name"     validate:"required"`
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type updateUserRequest struct {
	Name  string `json:"name"     validate:"required"`
	Email string `json:"email"    validate:"required,email"`
	// Password is optional on update; when provided it must be at least 8 characters.
	Password string `json:"password" validate:"omitempty,min=8"`
	// CurrentPassword re-authenticates a credential change (CON-193 §1): it is
	// required only when Password is present and is verified against the stored
	// hash, so it carries no min-length rule of its own. A plain name/email edit
	// leaves it empty and unchecked.
	CurrentPassword string `json:"current_password" validate:"required_with=Password"`
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
	user, err := h.repo.GetByIDWithTenant(c.Context(), session.UserID)
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
// @Description  Creates a new user in the authenticated caller's tenant (CON-97). Requires authentication; any tenant_id in the body is ignored.
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
	// New users always join the authenticated caller's tenant; a tenant_id in
	// the request body is never trusted (CON-97 §7.2, §12.2).
	session := c.Locals("session").(*models.Session)

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
		TenantID:     session.TenantID,
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: hash,
	}
	if err := h.repo.Create(c.Context(), user); err != nil {
		return err
	}

	h.activity.Record(c.Context(), activity.CategoryAuthentication, "user_created",
		activity.WithEntity("user", user.ID), activity.WithSource(activity.SourceAPI))
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
// @Description  Updates name and/or email of an existing user. When `password` is present the caller must also send `current_password`; it is re-verified and, on success, every other session for the user is revoked (the caller's own session is kept). CON-193.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string             true  "User Sqid"
// @Param        body  body      updateUserRequest  true  "User payload"
// @Success      200   {object}  models.User
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      403   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/users/{id} [put]
func (h *UsersHandler) Update(c *fiber.Ctx) error {
	if err := requireSelf(c); err != nil {
		return err
	}
	// requireSelf guarantees an authenticated session; hold on to it so a password
	// change can spare the caller's own session while revoking the rest (CON-193 §2).
	session := c.Locals("session").(*models.Session)

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

	changingPassword := req.Password != ""
	if changingPassword {
		// Re-authenticate before replacing the credential: a live session must prove
		// it knows the current password, so a borrowed tab or a stolen cookie can't
		// silently set a new one and lock the owner out (CON-193 §1). validate has
		// already ensured current_password is present (required_with=Password).
		ok, verr := models.VerifyPassword(req.CurrentPassword, user.PasswordHash)
		if verr != nil || !ok {
			return fiber.NewError(fiber.StatusForbidden, "current password is incorrect")
		}
		hash, herr := models.HashPassword(req.Password)
		if herr != nil {
			return herr
		}
		user.PasswordHash = hash
	}

	if changingPassword {
		// Persist the new credential and evict every other session in one transaction,
		// so the change can't half-apply and leave an intruder's session alive —
		// mirroring POST /api/password-reset/confirm. The caller's own session is
		// preserved so they aren't logged out of the tab making the change (CON-193 §2).
		if err := h.db.RunInTx(c.Context(), nil, func(ctx context.Context, tx bun.Tx) error {
			if _, err := tx.NewUpdate().Model(user).WherePK().Exec(ctx); err != nil {
				return err
			}
			_, err := tx.NewDelete().Model((*models.Session)(nil)).
				Where("user_id = ?", user.ID).
				Where("id != ?", session.ID).
				Exec(ctx)
			return err
		}); err != nil {
			return err
		}
	} else if err := h.repo.Update(c.Context(), user); err != nil {
		return err
	}

	h.activity.Record(c.Context(), activity.CategoryAuthentication, "user_updated",
		activity.WithEntity("user", user.ID), activity.WithSource(activity.SourceAPI))
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
	h.activity.Record(c.Context(), activity.CategoryAuthentication, "user_deleted",
		activity.WithEntity("user", c.Params("id")), activity.WithSource(activity.SourceAPI))
	return c.SendStatus(fiber.StatusNoContent)
}
