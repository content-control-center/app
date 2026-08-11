package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/activity"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// WorkspacesHandler serves the account-level workspace surface (CON-147): list
// the workspaces an account belongs to, create a new one, and choose the default
// the next fresh tab/login seeds into. These routes are account-scoped, not
// workspace-scoped — they act across the account's memberships — so unlike the
// content endpoints they do NOT read tenantctx; the active-workspace header
// resolved by RequireAuth is irrelevant here.
type WorkspacesHandler struct {
	db            *bun.DB
	workspaceRepo repository.WorkspaceRepository
	userRepo      repository.UserRepository
	accountRepo   repository.AccountRepository
	tenantRepo    repository.TenantRepository
	sessionRepo   repository.SessionRepository
	// profileJobs provisions a per-workspace Zernio profile on create, reusing the
	// signup bootstrap (CON-102). nil disables it (creation still succeeds).
	profileJobs ProfileBootstrapEnqueuer
	auth        fiber.Handler
	activity    *activity.Recorder
}

// NewWorkspacesHandler builds the handler.
func NewWorkspacesHandler(
	db *bun.DB,
	workspaceRepo repository.WorkspaceRepository,
	userRepo repository.UserRepository,
	accountRepo repository.AccountRepository,
	tenantRepo repository.TenantRepository,
	sessionRepo repository.SessionRepository,
	profileJobs ProfileBootstrapEnqueuer,
	auth fiber.Handler,
) *WorkspacesHandler {
	return &WorkspacesHandler{
		db:            db,
		workspaceRepo: workspaceRepo,
		userRepo:      userRepo,
		accountRepo:   accountRepo,
		tenantRepo:    tenantRepo,
		sessionRepo:   sessionRepo,
		profileJobs:   profileJobs,
		auth:          auth,
	}
}

// SetActivityRecorder wires the CON-125 activity recorder (nil-safe no-op).
func (h *WorkspacesHandler) SetActivityRecorder(r *activity.Recorder) { h.activity = r }

func (h *WorkspacesHandler) Register(app *fiber.App) {
	g := app.Group("/api/workspaces", h.auth)
	g.Get("/", h.List)
	g.Post("/", h.Create)
	g.Post("/:id/switch", h.Switch)
}

type createWorkspaceRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

// List godoc
// @Summary      List workspaces
// @Description  Returns every workspace the authenticated account is a member of, with the caller's role, the member count, and which one is the session default (CON-147).
// @Tags         workspaces
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   repository.WorkspaceListItem
// @Failure      401  {object}  map[string]string
// @Router       /api/workspaces [get]
func (h *WorkspacesHandler) List(c *fiber.Ctx) error {
	session := c.Locals("session").(*models.Session)
	items, err := h.workspaceRepo.ListForAccount(c.Context(), session.AccountID)
	if err != nil {
		return err
	}
	// Mark the stored default (RequireAuth stashed it before the active-workspace
	// override, so it survives an X-Workspace-Id on this very request).
	def, _ := c.Locals(DefaultWorkspaceLocal).(string)
	for i := range items {
		items[i].IsDefault = items[i].ID == def
	}
	return c.JSON(items)
}

// Create godoc
// @Summary      Create a workspace
// @Description  Creates a new, independent workspace owned by the authenticated account, provisions its Zernio profile, and returns it. Does not switch — the caller chooses when to move into it (CON-147).
// @Tags         workspaces
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      createWorkspaceRequest  true  "Workspace name"
// @Success      201   {object}  models.Tenant
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Router       /api/workspaces [post]
func (h *WorkspacesHandler) Create(c *fiber.Ctx) error {
	session := c.Locals("session").(*models.Session)

	var req createWorkspaceRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	// The membership is created for the caller's account, so it inherits the
	// account's display name/email (denormalised onto the users row, as signup
	// does).
	account, err := h.accountRepo.GetByID(c.Context(), session.AccountID)
	if err != nil {
		return err
	}

	slug, err := h.uniqueSlug(c.Context(), req.Name)
	if err != nil {
		return err
	}
	tenantID, err := models.NewID()
	if err != nil {
		return err
	}
	userID, err := models.NewID()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	tenant := &models.Tenant{ID: tenantID, Name: req.Name, Slug: slug, CreatedAt: now, UpdatedAt: now}
	// The creator owns the new workspace (CON-26 role model).
	membership := &models.User{ID: userID, AccountID: account.ID, TenantID: tenantID, Name: account.Name, Email: account.Email, Role: models.RoleOwner, CreatedAt: now, UpdatedAt: now}

	if err := h.db.RunInTx(c.Context(), nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(tenant).Exec(ctx); err != nil {
			return err
		}
		if err := h.userRepo.CreateTx(ctx, tx, membership); err != nil {
			return err
		}
		// Per-workspace Zernio profile, enqueued in-tx so the job exists iff the
		// workspace does — same pattern as signup (CON-102). Creation never blocks
		// on Zernio being reachable.
		if h.profileJobs != nil {
			if err := h.profileJobs.EnqueueBootstrapProfileTx(ctx, tx.Tx, tenantID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if isUniqueViolation(err) {
			// Lost a slug race; the caller can retry.
			return fiber.NewError(fiber.StatusConflict, "could not allocate a unique workspace slug")
		}
		return err
	}

	h.activity.Record(
		logging.WithUserID(tenantctx.With(c.Context(), tenantID), userID),
		activity.CategoryWorkspace, "workspace_created",
		activity.WithEntity("tenant", tenantID), activity.WithSource(activity.SourceAPI),
	)

	return c.Status(fiber.StatusCreated).JSON(tenant)
}

// Switch godoc
// @Summary      Set the default workspace
// @Description  Repoints the session's default workspace — where a fresh tab or the next login seeds. It does not rescope open tabs (each carries its own X-Workspace-Id), so switching in one tab leaves the others alone (CON-147). 403 if the account isn't a member of the target.
// @Tags         workspaces
// @Security     CookieAuth
// @Param        id  path  string  true  "Workspace id"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Router       /api/workspaces/{id}/switch [post]
func (h *WorkspacesHandler) Switch(c *fiber.Ctx) error {
	session := c.Locals("session").(*models.Session)
	targetID := c.Params("id")

	// Only a workspace the account belongs to can become its default.
	membership, err := h.userRepo.GetMembership(c.Context(), session.AccountID, targetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// 404 (not 403) so the endpoint doesn't confirm a workspace the account
			// can't see even exists (CON-97 §12.3).
			return fiber.NewError(fiber.StatusNotFound, "workspace not found")
		}
		return err
	}

	if err := h.sessionRepo.SetDefaultWorkspace(c.Context(), session.ID, membership.ID, targetID); err != nil {
		return err
	}

	h.activity.Record(
		logging.WithUserID(tenantctx.With(c.Context(), targetID), membership.ID),
		activity.CategoryWorkspace, "workspace_switched",
		activity.WithEntity("tenant", targetID), activity.WithSource(activity.SourceAPI),
	)

	return c.SendStatus(fiber.StatusNoContent)
}

// uniqueSlug returns slugify(name) suffixed with -2, -3, … until free. Mirrors
// TenantsHandler.uniqueSlug so a workspace created while logged in slugs exactly
// like one created at signup.
func (h *WorkspacesHandler) uniqueSlug(ctx context.Context, name string) (string, error) {
	base := slugify(name)
	slug := base
	for i := 2; i <= 1000; i++ {
		_, err := h.tenantRepo.GetBySlug(ctx, slug)
		if errors.Is(err, sql.ErrNoRows) {
			return slug, nil
		}
		if err != nil {
			return "", err
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	return "", fiber.NewError(fiber.StatusConflict, "could not allocate a unique slug")
}
