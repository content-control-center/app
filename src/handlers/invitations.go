package handlers

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/activity"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// Invitation rate-limit budget (CON-26). Creating an invite sends mail on
// demand, so it is throttled per workspace (so one workspace can't be used to
// blast mail) and per client IP (so one caller can't spray across workspaces).
// Accept is public and does argon2 work, so it is throttled per IP to blunt
// token-guessing — the tokens are 256-bit, so this is defence-in-depth.
const (
	invitePerTenantBurst = 20
	invitePerIPBurst     = 30
	inviteAcceptPerIP    = 30
	inviteRateWindow     = time.Hour
)

// inviteThrottledMsg / invitationInvalidMsg are surfaced to the client verbatim.
// The invalid message is deliberately one indistinguishable sentence for every
// unusable token — unknown, expired, revoked, or already accepted — so accept is
// not an oracle (mirrors the password-reset contract, CON-161).
const (
	inviteThrottledMsg   = "Too many invitations. Please wait a minute and try again."
	invitationInvalidMsg = "This invitation link is invalid or has expired."
)

// errInvitationInvalid is the internal sentinel the accept transaction returns
// for any unusable token; it maps to a 410 + invitationInvalidMsg.
var errInvitationInvalid = errors.New("invitation invalid")

// InvitationEmailEnqueuer enqueues the transactional invitation email inside the
// invite-minting transaction, so the mail exists iff the invitation row does
// (CON-26). Implemented by *queues.Enqueuer; a narrow interface keeps this
// handler out of the jobs package and unit-testable.
type InvitationEmailEnqueuer interface {
	EnqueueInvitationEmailTx(ctx context.Context, tx *sql.Tx, tenantID, invitationID, toEmail, inviteURL, inviterName, workspaceName, role string) error
}

// InvitationsHandler serves workspace invitations (CON-26): owner-gated
// create/list/revoke, and the public preview/accept the invitee uses. Accepting
// creates a users row in the invite's tenant and opens a session — Ogen stays
// one-user-one-tenant (the multi-workspace account split is CON-147).
type InvitationsHandler struct {
	db           *bun.DB
	userRepo     repository.UserRepository
	tenantRepo   repository.TenantRepository
	inviteRepo   repository.InvitationRepository
	sessionRepo  repository.SessionRepository
	emailJobs    InvitationEmailEnqueuer
	appBaseURL   string
	cookieName   string
	secureCookie bool
	auth         fiber.Handler

	tenantLimiter *keyedRateLimiter
	ipLimiter     *keyedRateLimiter
	acceptLimiter *keyedRateLimiter

	activity *activity.Recorder
}

// NewInvitationsHandler builds the handler. appBaseURL (APP_BASE_URL) is the base
// for the emailed accept link; cookieName/secureCookie mirror the session cookie
// set at login so accept can auto-log-in the new user.
func NewInvitationsHandler(db *bun.DB, userRepo repository.UserRepository, tenantRepo repository.TenantRepository, inviteRepo repository.InvitationRepository, sessionRepo repository.SessionRepository, appBaseURL, cookieName string, secureCookie bool, auth fiber.Handler) *InvitationsHandler {
	return &InvitationsHandler{
		db:            db,
		userRepo:      userRepo,
		tenantRepo:    tenantRepo,
		inviteRepo:    inviteRepo,
		sessionRepo:   sessionRepo,
		appBaseURL:    appBaseURL,
		cookieName:    cookieName,
		secureCookie:  secureCookie,
		auth:          auth,
		tenantLimiter: newKeyedRateLimiter(invitePerTenantBurst, inviteRateWindow),
		ipLimiter:     newKeyedRateLimiter(invitePerIPBurst, inviteRateWindow),
		acceptLimiter: newKeyedRateLimiter(inviteAcceptPerIP, inviteRateWindow),
	}
}

// SetActivityRecorder wires the CON-125 activity recorder (nil-safe no-op).
func (h *InvitationsHandler) SetActivityRecorder(r *activity.Recorder) { h.activity = r }

// SetEmailEnqueuer wires the CON-26 invitation-email enqueuer (nil-safe no-op).
// When unset, creating an invite mints + stores it but sends no mail.
func (h *InvitationsHandler) SetEmailEnqueuer(e InvitationEmailEnqueuer) { h.emailJobs = e }

func (h *InvitationsHandler) Register(app *fiber.App) {
	// Public accept surface — the emailed token is the capability, so these are
	// unauthenticated. Registered BEFORE the auth-gated group below: the group's
	// auth middleware is matched by the /api/invitations prefix, so anything
	// registered after it under that prefix would inherit auth (mirrors the
	// signup-before-group ordering in tenants.go).
	app.Get("/api/invitations/accept/:token", h.Preview)
	app.Post("/api/invitations/accept/:token", h.Accept)

	// Management: owner-gated inside each handler (via requireOwner).
	g := app.Group("/api/invitations", h.auth)
	g.Post("/", h.Create)
	g.Get("/", h.List)
	g.Delete("/:id", h.Revoke)
}

type createInvitationRequest struct {
	Email string `json:"email" validate:"required,email"`
	// Role defaults to member; an owner may invite a co-owner by passing "owner".
	Role string `json:"role" validate:"omitempty,oneof=owner member"`
}

// Create godoc
// @Summary      Invite a teammate
// @Description  Owner-only. Emails a single-use invitation link to join the caller's workspace with the given role (default member). Returns 409 if the email already has an Ogen account anywhere, or if a live invite for it is already pending. Rate-limited per workspace and per IP (CON-26).
// @Tags         invitations
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      createInvitationRequest  true  "Invitation payload"
// @Success      201   {object}  models.Invitation
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      403   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Failure      429   {object}  map[string]string
// @Router       /api/invitations [post]
func (h *InvitationsHandler) Create(c *fiber.Ctx) error {
	caller, err := requireOwner(c, h.userRepo)
	if err != nil {
		return err
	}

	var req createInvitationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}
	email := repository.NormalizeEmail(req.Email)
	role := req.Role
	if role == "" {
		role = models.RoleMember
	}
	tenantID := caller.TenantID

	// Each invite sends mail, so throttle before doing the work. IP first, so a
	// throttled caller doesn't also spend the workspace's budget.
	if ok, retry := h.ipLimiter.allow(c.IP()); !ok {
		return tooManyRequests(c, retry, inviteThrottledMsg)
	}
	if ok, retry := h.tenantLimiter.allow(tenantID); !ok {
		return tooManyRequests(c, retry, inviteThrottledMsg)
	}

	// An email already tied to an Ogen account anywhere can't be invited — the
	// global-unique users.email forbids a second row (CON-26; cross-workspace
	// membership is CON-147). The caller is an authenticated owner, so surfacing
	// "already an account" is acceptable (unlike the public reset endpoint).
	if _, err := h.userRepo.GetByEmail(c.Context(), email); err == nil {
		return fiber.NewError(fiber.StatusConflict, "this email already has an Ogen account")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := time.Now().UTC()
	// One live invite per address per workspace (the partial unique index is the
	// hard backstop; this is the friendly pre-check). A pending row whose
	// expires_at has already passed is treated as expired: it still occupies the
	// partial-unique (pending) slot, so it is cleared inside the tx below rather
	// than blocking a fresh invite. Only a still-active pending invite is a 409.
	if existing, err := h.inviteRepo.GetPendingByTenantEmail(c.Context(), tenantID, email); err == nil {
		if now.Before(existing.ExpiresAt) {
			return fiber.NewError(fiber.StatusConflict, "an invitation for this email is already pending")
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	token, hash, err := models.NewInvitationToken()
	if err != nil {
		return err
	}
	id, err := models.NewID()
	if err != nil {
		return err
	}
	inv := &models.Invitation{
		ID:        id,
		TenantID:  tenantID,
		Email:     email,
		Role:      role,
		TokenHash: hash,
		InvitedBy: caller.ID,
		Status:    models.InvitationPending,
		ExpiresAt: now.Add(models.InvitationTTL),
		CreatedAt: now,
	}
	inviteURL := strings.TrimRight(h.appBaseURL, "/") + "/invite?token=" + token

	// The email needs the workspace name; the invitee has no user/tenant to load
	// at send time, so resolve it here and pass it as a template var.
	workspaceName := ""
	if t, terr := h.tenantRepo.GetByID(c.Context(), tenantID); terr == nil && t != nil {
		workspaceName = t.Name
	}

	// Invite row + email enqueue commit together: a rolled-back tx queues no mail,
	// a committed one queues exactly one link resolving to a stored hash (mirrors
	// password-reset dispatch, CON-161). CreateReplacingExpiredTx also clears any
	// expired pending invite that would otherwise hold the partial-unique slot.
	if err := h.db.RunInTx(c.Context(), nil, func(ctx context.Context, tx bun.Tx) error {
		if err := h.inviteRepo.CreateReplacingExpiredTx(ctx, tx, inv); err != nil {
			return err
		}
		if h.emailJobs != nil {
			return h.emailJobs.EnqueueInvitationEmailTx(ctx, tx.Tx, tenantID, id, email, inviteURL, caller.Name, workspaceName, role)
		}
		return nil
	}); err != nil {
		// A concurrent create for the same address loses the race to the partial
		// unique index; surface that as 409, not a raw 500.
		if isUniqueViolation(err) {
			return fiber.NewError(fiber.StatusConflict, "an invitation for this email is already pending")
		}
		return err
	}

	h.activity.Record(
		logging.WithUserID(tenantctx.With(c.Context(), tenantID), caller.ID),
		activity.CategoryAuthentication, "member_invited",
		activity.WithEntity("invitation", id), activity.WithSource(activity.SourceAPI),
		activity.WithPayload(map[string]any{"role": role}),
	)
	return c.Status(fiber.StatusCreated).JSON(inv)
}

// List godoc
// @Summary      List invitations
// @Description  Owner-only. Lists the caller's workspace invitations, newest first (CON-26).
// @Tags         invitations
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   models.Invitation
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Router       /api/invitations [get]
func (h *InvitationsHandler) List(c *fiber.Ctx) error {
	caller, err := requireOwner(c, h.userRepo)
	if err != nil {
		return err
	}
	invs, err := h.inviteRepo.ListByTenant(c.Context(), caller.TenantID)
	if err != nil {
		return err
	}
	return c.JSON(invs)
}

// Revoke godoc
// @Summary      Revoke an invitation
// @Description  Owner-only. Revokes a pending invitation in the caller's workspace, killing its link (CON-26).
// @Tags         invitations
// @Security     CookieAuth
// @Param        id   path  string  true  "Invitation id"
// @Success      204
// @Failure      401  {object}  map[string]string
// @Failure      403  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/invitations/{id} [delete]
func (h *InvitationsHandler) Revoke(c *fiber.Ctx) error {
	caller, err := requireOwner(c, h.userRepo)
	if err != nil {
		return err
	}
	ok, err := h.inviteRepo.Revoke(c.Context(), c.Params("id"), caller.TenantID)
	if err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "invitation not found")
	}
	h.activity.Record(
		logging.WithUserID(tenantctx.With(c.Context(), caller.TenantID), caller.ID),
		activity.CategoryAuthentication, "invitation_revoked",
		activity.WithEntity("invitation", c.Params("id")), activity.WithSource(activity.SourceAPI),
	)
	return c.SendStatus(fiber.StatusNoContent)
}

type invitationPreviewResponse struct {
	WorkspaceName string    `json:"workspace_name"`
	InviterName   string    `json:"inviter_name"`
	Email         string    `json:"email"`
	Role          string    `json:"role"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// Preview godoc
// @Summary      Preview an invitation
// @Description  Public. Returns the workspace, inviter, invited email and role for a valid, pending, unexpired invitation token; every unusable token returns the same generic 410 so the endpoint is not an oracle (CON-26).
// @Tags         invitations
// @Produce      json
// @Param        token  path  string  true  "Invitation token"
// @Success      200  {object}  invitationPreviewResponse
// @Failure      410  {object}  map[string]string
// @Router       /api/invitations/accept/{token} [get]
func (h *InvitationsHandler) Preview(c *fiber.Ctx) error {
	inv, err := h.inviteRepo.GetByTokenHash(c.Context(), models.HashInvitationToken(c.Params("token")))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Unknown, expired, and revoked tokens all answer with the same 410 +
			// message (matching Accept), so preview isn't a token-existence oracle.
			return fiber.NewError(fiber.StatusGone, invitationInvalidMsg)
		}
		return err
	}
	if inv.Status != models.InvitationPending || time.Now().UTC().After(inv.ExpiresAt) {
		return fiber.NewError(fiber.StatusGone, invitationInvalidMsg)
	}

	// The invite authorizes reading its own workspace's display data; scope the
	// tenant-scoped reads to the invite's tenant. Both are best-effort — a missing
	// name just renders blank on the accept page.
	ctx := tenantctx.With(c.Context(), inv.TenantID)
	resp := invitationPreviewResponse{Email: inv.Email, Role: inv.Role, ExpiresAt: inv.ExpiresAt}
	if t, terr := h.tenantRepo.GetByID(ctx, inv.TenantID); terr == nil && t != nil {
		resp.WorkspaceName = t.Name
	}
	if u, uerr := h.userRepo.GetByID(ctx, inv.InvitedBy); uerr == nil && u != nil {
		resp.InviterName = u.Name
	}
	return c.JSON(resp)
}

type acceptInvitationRequest struct {
	Name     string `json:"name"     validate:"required"`
	Password string `json:"password" validate:"required,min=8"`
}

type acceptInvitationResponse struct {
	Tenant  *models.Tenant  `json:"tenant,omitempty"`
	User    *models.User    `json:"user"`
	Session *models.Session `json:"session"`
}

// Accept godoc
// @Summary      Accept an invitation
// @Description  Public. Consumes a single-use invitation token, creates the invited user in the workspace with the invited role, opens a session (auto-login), and sets the session cookie (CON-26). Returns a generic 410 for any unusable token, and 409 if the email gained an account since the invite was sent.
// @Tags         invitations
// @Accept       json
// @Produce      json
// @Param        token  path  string                    true  "Invitation token"
// @Param        body   body  acceptInvitationRequest   true  "Name + password"
// @Success      201  {object}  acceptInvitationResponse
// @Failure      400  {object}  map[string]string
// @Failure      409  {object}  map[string]string
// @Failure      410  {object}  map[string]string
// @Failure      429  {object}  map[string]string
// @Router       /api/invitations/accept/{token} [post]
func (h *InvitationsHandler) Accept(c *fiber.Ctx) error {
	// Throttle the public, argon2-doing path per IP before any work.
	if ok, retry := h.acceptLimiter.allow(c.IP()); !ok {
		return tooManyRequests(c, retry, inviteThrottledMsg)
	}

	var req acceptInvitationRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	// Validate BEFORE consuming the token, so a too-short password doesn't burn a
	// still-valid invite — the invitee can retry it (mirrors password-reset confirm).
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	tokenHash := models.HashInvitationToken(c.Params("token"))
	now := time.Now().UTC()

	var (
		newUser  *models.User
		session  *models.Session
		tenantID string
	)
	err := h.db.RunInTx(c.Context(), nil, func(ctx context.Context, tx bun.Tx) error {
		// Atomically spend the invite (unknown/expired/revoked/already-accepted all
		// come back as ErrNoRows → one indistinguishable 410).
		inv, err := h.inviteRepo.ConsumeByTokenTx(ctx, tx, tokenHash, now)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errInvitationInvalid
			}
			return err
		}

		uid, err := models.NewID()
		if err != nil {
			return err
		}
		hash, err := models.HashPassword(req.Password)
		if err != nil {
			return err
		}
		newUser = &models.User{
			ID:           uid,
			TenantID:     inv.TenantID,
			Name:         req.Name,
			Email:        inv.Email,
			PasswordHash: hash,
			Role:         inv.Role,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		// If the address gained an account between invite and accept, the users
		// unique constraint fires here and the whole tx rolls back — the invite is
		// left unconsumed (accept is retryable once the conflict is resolved).
		if err := h.userRepo.CreateTx(ctx, tx, newUser); err != nil {
			return err
		}

		tokenStr, err := models.NewSessionToken()
		if err != nil {
			return err
		}
		session = &models.Session{
			ID:        tokenStr,
			UserID:    uid,
			TenantID:  inv.TenantID,
			ExpiresAt: now.Add(sessionTTL),
			CreatedAt: now,
		}
		if err := h.sessionRepo.CreateTx(ctx, tx, session); err != nil {
			return err
		}
		tenantID = inv.TenantID
		return nil
	})
	if err != nil {
		if errors.Is(err, errInvitationInvalid) {
			return fiber.NewError(fiber.StatusGone, invitationInvalidMsg)
		}
		if isUniqueViolation(err) {
			return fiber.NewError(fiber.StatusConflict, "this email already has an Ogen account")
		}
		return err
	}

	c.Cookie(&fiber.Cookie{
		Name:     h.cookieName,
		Value:    session.ID,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HTTPOnly: true,
		Secure:   h.secureCookie,
		SameSite: "Lax",
	})

	h.activity.Record(
		logging.WithUserID(tenantctx.With(c.Context(), tenantID), newUser.ID),
		activity.CategoryAuthentication, "invitation_accepted",
		activity.WithEntity("user", newUser.ID), activity.WithSource(activity.SourceAPI),
	)

	resp := acceptInvitationResponse{User: newUser, Session: session}
	if t, terr := h.tenantRepo.GetByID(c.Context(), tenantID); terr == nil {
		resp.Tenant = t
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}
