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
	g.Post("/", h.auth, h.Create)           // owner-only; new users join the caller's tenant (CON-26/97)
	g.Get("/", h.auth, h.List)              // always protected
	g.Get("/:id", h.auth, h.Get)            // always protected
	g.Put("/:id", h.auth, h.Update)         // always protected
	g.Patch("/:id/role", h.auth, h.SetRole) // owner-only role change (CON-26)
	g.Delete("/:id", h.auth, h.Delete)      // self or owner (CON-26)
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
	// Role is optional; it defaults to member. Only an owner may create users
	// (CON-26), so an owner can also mint a co-owner by passing "owner".
	Role string `json:"role" validate:"omitempty,oneof=owner member"`
}

// setRoleRequest is the body of PATCH /api/users/:id/role (CON-26).
type setRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=owner member"`
}

// errLastOwner is returned from inside a role-change / removal transaction when
// the operation would leave the workspace with no owner. Mapped to 409 — a
// workspace must always keep at least one owner (CON-26 §7).
var errLastOwner = errors.New("cannot remove or demote the last owner")

// errCurrentPasswordMismatch is returned from inside the password-change
// transaction when the supplied current password fails re-verification, so the
// tx rolls back and the handler can map it to a 403 (CON-193 §1). Mirrors
// errResetInvalid in password_reset.go.
var errCurrentPasswordMismatch = errors.New("current password is incorrect")

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
// @Description  Creates a new user in the authenticated caller's tenant (CON-97). Owner-only (CON-26); the new user defaults to the member role. Any tenant_id in the body is ignored. Email-based invitations (POST /api/invitations) are the preferred way to add teammates.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      createUserRequest  true  "User payload"
// @Success      201   {object}  models.User
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      403   {object}  map[string]string
// @Router       /api/users [post]
func (h *UsersHandler) Create(c *fiber.Ctx) error {
	// Only an owner may add users directly (CON-26). Their tenant is the target;
	// a tenant_id in the request body is never trusted (CON-97 §7.2, §12.2).
	caller, err := requireOwner(c, h.repo)
	if err != nil {
		return err
	}

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

	role := req.Role
	if role == "" {
		role = models.RoleMember
	}

	user := &models.User{
		ID:           id,
		TenantID:     caller.TenantID,
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: hash,
		Role:         role,
	}
	if err := h.repo.Create(c.Context(), user); err != nil {
		return err
	}

	h.activity.Record(c.Context(), activity.CategoryAuthentication, "user_created",
		activity.WithEntity("user", user.ID), activity.WithSource(activity.SourceAPI))
	return c.Status(fiber.StatusCreated).JSON(user)
}

// SetRole godoc
// @Summary      Change a member's role
// @Description  Owner-only. Sets a workspace member's role to owner or member (CON-26). The workspace must always keep at least one owner, so demoting the last owner returns 409.
// @Tags         users
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string          true  "User Sqid"
// @Param        body  body      setRoleRequest  true  "Role payload"
// @Success      200   {object}  models.User
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      403   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Router       /api/users/{id}/role [patch]
func (h *UsersHandler) SetRole(c *fiber.Ctx) error {
	caller, err := requireOwner(c, h.repo)
	if err != nil {
		return err
	}

	var req setRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	// GetByID is tenant-scoped, so this 404s for a user outside the caller's
	// workspace — an owner can only touch their own members (CON-26 §11).
	target, err := h.repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "user not found")
		}
		return err
	}
	if target.Role == req.Role {
		return c.JSON(target) // no-op
	}

	// Demoting an owner must not orphan the workspace; the guard + update run in
	// one transaction that locks the owner rows, so two concurrent demotions can't
	// both pass and leave zero owners.
	if err := h.db.RunInTx(c.Context(), nil, func(ctx context.Context, tx bun.Tx) error {
		if target.Role == models.RoleOwner && req.Role != models.RoleOwner {
			if err := ensureNotLastOwner(ctx, tx, target.TenantID); err != nil {
				return err
			}
		}
		_, err := tx.NewUpdate().Model((*models.User)(nil)).
			Set("role = ?", req.Role).
			Set("updated_at = ?", time.Now().UTC()).
			Where("id = ?", target.ID).
			Where("tenant_id = ?", target.TenantID).
			Exec(ctx)
		return err
	}); err != nil {
		if errors.Is(err, errLastOwner) {
			return fiber.NewError(fiber.StatusConflict, "a workspace must have at least one owner")
		}
		return err
	}
	target.Role = req.Role

	h.activity.Record(c.Context(), activity.CategoryAuthentication, "member_role_changed",
		activity.WithEntity("user", target.ID), activity.WithSource(activity.SourceAPI),
		activity.WithPayload(map[string]any{"role": req.Role, "actor": caller.ID}))
	return c.JSON(target)
}

// ensureNotLastOwner returns errLastOwner unless the tenant would still have an
// owner after the caller demotes or removes one. It locks the tenant's owner
// rows (FOR UPDATE) so concurrent demotions/removals serialize and can't race
// the workspace down to zero owners (CON-26 §7). Must be called inside a tx.
func ensureNotLastOwner(ctx context.Context, tx bun.Tx, tenantID string) error {
	var owners []models.User
	if err := tx.NewSelect().Model(&owners).Column("id").
		Where("tenant_id = ?", tenantID).
		Where("role = ?", models.RoleOwner).
		For("UPDATE").
		Scan(ctx); err != nil {
		return err
	}
	if len(owners) <= 1 {
		return errLastOwner
	}
	return nil
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

	if req.Password == "" {
		// Name/email-only edit: no re-authentication and no session revocation.
		user.Name = req.Name
		user.Email = req.Email
		user.UpdatedAt = time.Now().UTC()
		if err := h.repo.Update(c.Context(), user); err != nil {
			return err
		}
	} else {
		// Password change. Lock the user row, re-verify the current password against
		// the *locked* hash, rotate it, and revoke the user's other sessions — all in
		// one transaction. FOR UPDATE serializes concurrent changes, so an in-flight
		// request carrying the old (possibly compromised) credential can't verify
		// against a stale hash and slip through after a rotation has already committed
		// (CON-193 §1/§2). Mirrors POST /api/password-reset/confirm, which likewise
		// holds the row lock across argon2 — password changes are rare, so hashing
		// under the lock is fine. The caller's own session (session.ID) is preserved
		// so they aren't logged out of the tab making the change.
		if err := h.db.RunInTx(c.Context(), nil, func(ctx context.Context, tx bun.Tx) error {
			if err := tx.NewSelect().Model(user).WherePK().For("UPDATE").Scan(ctx); err != nil {
				return err
			}
			ok, verr := models.VerifyPassword(req.CurrentPassword, user.PasswordHash)
			if verr != nil {
				return verr
			}
			if !ok {
				return errCurrentPasswordMismatch
			}
			hash, herr := models.HashPassword(req.Password)
			if herr != nil {
				return herr
			}
			user.Name = req.Name
			user.Email = req.Email
			user.PasswordHash = hash
			user.UpdatedAt = time.Now().UTC()
			if _, err := tx.NewUpdate().Model(user).WherePK().Exec(ctx); err != nil {
				return err
			}
			_, err := tx.NewDelete().Model((*models.Session)(nil)).
				Where("user_id = ?", user.ID).
				Where("id != ?", session.ID).
				Exec(ctx)
			return err
		}); err != nil {
			if errors.Is(err, errCurrentPasswordMismatch) {
				return fiber.NewError(fiber.StatusForbidden, "current password is incorrect")
			}
			return err
		}
	}

	h.activity.Record(c.Context(), activity.CategoryAuthentication, "user_updated",
		activity.WithEntity("user", user.ID), activity.WithSource(activity.SourceAPI))
	return c.JSON(user)
}

// Delete godoc
// @Summary      Delete user
// @Description  Removes a workspace member. A user may remove themselves; an owner may remove anyone in the workspace (CON-26). The workspace must always keep at least one owner, so removing the last owner (including self) returns 409.
// @Tags         users
// @Security     CookieAuth
// @Param        id   path  string  true  "User Sqid"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Router       /api/users/{id} [delete]
func (h *UsersHandler) Delete(c *fiber.Ctx) error {
	caller, err := callerUser(c, h.repo)
	if err != nil {
		return err
	}
	targetID := c.Params("id")
	// A member may only remove themselves; removing anyone else requires owner.
	if caller.ID != targetID && caller.Role != models.RoleOwner {
		return fiber.NewError(fiber.StatusForbidden, "forbidden")
	}

	// Tenant-scoped, so an owner can't reach into another workspace's user.
	target, err := h.repo.GetByID(c.Context(), targetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "user not found")
		}
		return err
	}

	// Removing an owner (including self-removal) must not orphan the workspace;
	// the guard + delete run in one owner-row-locking transaction.
	if err := h.db.RunInTx(c.Context(), nil, func(ctx context.Context, tx bun.Tx) error {
		if target.Role == models.RoleOwner {
			if err := ensureNotLastOwner(ctx, tx, target.TenantID); err != nil {
				return err
			}
		}
		_, err := tx.NewDelete().Model((*models.User)(nil)).
			Where("id = ?", target.ID).
			Where("tenant_id = ?", target.TenantID).
			Exec(ctx)
		return err
	}); err != nil {
		if errors.Is(err, errLastOwner) {
			return fiber.NewError(fiber.StatusConflict, "a workspace must have at least one owner")
		}
		return err
	}

	// Distinguish self-removal from an owner removing a teammate (CON-26 §13).
	event := "user_deleted"
	if caller.ID != targetID {
		event = "member_removed"
	}
	h.activity.Record(c.Context(), activity.CategoryAuthentication, event,
		activity.WithEntity("user", targetID), activity.WithSource(activity.SourceAPI))
	return c.SendStatus(fiber.StatusNoContent)
}
