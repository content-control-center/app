package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/activity"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// ProfileBootstrapEnqueuer enqueues an eager Zernio profile-provisioning job in
// the caller's transaction, so the job commits atomically with the new tenant
// (CON-102 §6 FR2). Implemented by *queues.Enqueuer; kept as a narrow interface
// here so the handler doesn't import the jobs package and stays unit-testable.
type ProfileBootstrapEnqueuer interface {
	EnqueueBootstrapProfileTx(ctx context.Context, tx *sql.Tx, tenantID string) error
}

// EmailEnqueuer enqueues the CON-154 lifecycle emails (welcome + onboarding
// drip) in the signup transaction, so they commit atomically with the new
// tenant/user. Implemented by *queues.Enqueuer; a narrow interface here keeps
// the handler out of the jobs package and unit-testable.
type EmailEnqueuer interface {
	EnqueueWelcomeEmailTx(ctx context.Context, tx *sql.Tx, userID, tenantID string) error
	EnqueueDripTx(ctx context.Context, tx *sql.Tx, userID, tenantID string) error
}

// Signup throttle budget (CON-162). POST /api/tenants is open and unauthenticated,
// so it is throttled per client IP to blunt automated mass account creation.
// Unlike login, every attempt is charged (a successful signup is exactly the
// abuse being limited), and there is no per-address dimension — each signup uses
// a fresh address, so the IP is the only meaningful key.
const (
	signupPerIPBurst = 10
	signupRateWindow = time.Hour
)

// signupThrottledMsg is the 429 body for a throttled signup; the signup form
// surfaces it verbatim.
const signupThrottledMsg = "Too many signup attempts. Please wait a minute and try again."

// TenantsHandler owns tenant provisioning (public self-service signup) and the
// tenant CRU surface (no delete). Tenants are the isolation boundary, so the
// read/update endpoints only ever operate on the caller's own tenant (CON-97).
type TenantsHandler struct {
	db           *bun.DB
	tenantRepo   repository.TenantRepository
	userRepo     repository.UserRepository
	profileJobs  ProfileBootstrapEnqueuer
	emailJobs    EmailEnqueuer
	cookieName   string
	secureCookie bool
	auth         fiber.Handler
	// ipLimiter throttles signup per client IP (CON-162).
	ipLimiter *keyedRateLimiter
	// activity records CON-125 authentication events (signup, tenant_updated).
	// Signup runs outside tenant scope, so it builds an explicit context. nil is
	// a no-op. Wired via SetActivityRecorder.
	activity *activity.Recorder
}

func NewTenantsHandler(db *bun.DB, tenantRepo repository.TenantRepository, userRepo repository.UserRepository, profileJobs ProfileBootstrapEnqueuer, cookieName string, secureCookie bool, auth fiber.Handler) *TenantsHandler {
	return &TenantsHandler{
		db:           db,
		tenantRepo:   tenantRepo,
		userRepo:     userRepo,
		profileJobs:  profileJobs,
		cookieName:   cookieName,
		secureCookie: secureCookie,
		auth:         auth,
		ipLimiter:    newKeyedRateLimiter(signupPerIPBurst, signupRateWindow),
	}
}

// SetActivityRecorder wires the CON-125 activity recorder (nil-safe no-op).
func (h *TenantsHandler) SetActivityRecorder(r *activity.Recorder) { h.activity = r }

// SetEmailEnqueuer wires the CON-154 lifecycle-email enqueuer (nil-safe no-op).
// When unset, signup simply sends no welcome/drip mail.
func (h *TenantsHandler) SetEmailEnqueuer(e EmailEnqueuer) { h.emailJobs = e }

func (h *TenantsHandler) Register(app *fiber.App) {
	app.Post("/api/tenants", h.Signup) // public self-service signup

	g := app.Group("/api/tenants", h.auth)
	g.Get("/current", h.Current)
	g.Get("/:id", h.Get)
	g.Put("/:id", h.Update)
}

type signupRequest struct {
	Tenant struct {
		Name string `json:"name" validate:"required"`
	} `json:"tenant"`
	User struct {
		Name     string `json:"name"     validate:"required"`
		Email    string `json:"email"    validate:"required,email"`
		Password string `json:"password" validate:"required,min=8"`
	} `json:"user"`
}

type signupResponse struct {
	Tenant  *models.Tenant  `json:"tenant"`
	User    *models.User    `json:"user"`
	Session *models.Session `json:"session"`
}

// Signup godoc
// @Summary      Sign up (create tenant)
// @Description  Public self-service signup: atomically creates a tenant and its first user, opens a session, and returns the session cookie (CON-97 §7.1).
// @Tags         tenants
// @Accept       json
// @Produce      json
// @Param        body  body      signupRequest  true  "Signup payload"
// @Success      201   {object}  signupResponse
// @Failure      400   {object}  map[string]string
// @Failure      409   {object}  map[string]string
// @Router       /api/tenants [post]
func (h *TenantsHandler) Signup(c *fiber.Ctx) error {
	var req signupRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	// Throttle mass signup per client IP (CON-162). Every well-formed attempt is
	// charged — a successful signup is the abuse being limited — so a spike from
	// one source answers 429 with a Retry-After once the budget is spent.
	if ok, retry := h.ipLimiter.allow(c.IP()); !ok {
		return tooManyRequests(c, retry, signupThrottledMsg)
	}

	// Email is globally unique (login is by email -> user -> tenant). Reject a
	// duplicate up front with 409 (PRD §9); the unique constraint is the backstop.
	if _, err := h.userRepo.GetByEmail(c.Context(), req.User.Email); err == nil {
		return fiber.NewError(fiber.StatusConflict, "email already in use")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	slug, err := h.uniqueSlug(c.Context(), req.Tenant.Name)
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
	hash, err := models.HashPassword(req.User.Password)
	if err != nil {
		return err
	}
	token, err := models.NewSessionToken()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	tenant := &models.Tenant{ID: tenantID, Name: req.Tenant.Name, Slug: slug, CreatedAt: now, UpdatedAt: now}
	user := &models.User{ID: userID, TenantID: tenantID, Name: req.User.Name, Email: req.User.Email, PasswordHash: hash, CreatedAt: now, UpdatedAt: now}
	session := &models.Session{ID: token, UserID: userID, TenantID: tenantID, ExpiresAt: now.Add(sessionTTL), CreatedAt: now}

	if err := h.db.RunInTx(c.Context(), nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewInsert().Model(tenant).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(user).Exec(ctx); err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(session).Exec(ctx); err != nil {
			return err
		}
		// CON-102: eagerly provision this tenant's Zernio profile in the
		// background, enqueued inside THIS tx so the job exists iff the tenant
		// does. The enqueue is a local DB insert (River shares bun's pool); the
		// Zernio call happens later in the worker, so signup never blocks on
		// Zernio reachability.
		if h.profileJobs != nil {
			if err := h.profileJobs.EnqueueBootstrapProfileTx(ctx, tx.Tx, tenantID); err != nil {
				return err
			}
		}
		// CON-154: welcome (immediate) + onboarding drip (day 2/5/7) enqueued in
		// this same tx, so a rolled-back signup queues no mail and a committed one
		// durably queues exactly one welcome + drip. The enqueues are local DB
		// inserts — the Resend calls happen later in the worker — so signup never
		// blocks on Resend reachability.
		if h.emailJobs != nil {
			if err := h.emailJobs.EnqueueWelcomeEmailTx(ctx, tx.Tx, userID, tenantID); err != nil {
				return err
			}
			if err := h.emailJobs.EnqueueDripTx(ctx, tx.Tx, userID, tenantID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		// A concurrent signup with the same email passes the pre-check above
		// but loses the race to the users.email unique constraint; surface that
		// as 409 rather than a raw 500 (TOCTOU backstop — the constraint is the
		// real source of truth).
		if isUniqueViolation(err) {
			return fiber.NewError(fiber.StatusConflict, "email already in use")
		}
		return err
	}

	c.Cookie(&fiber.Cookie{
		Name:     h.cookieName,
		Value:    token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HTTPOnly: true,
		Secure:   h.secureCookie,
		SameSite: "Lax",
	})

	h.activity.Record(
		logging.WithUserID(tenantctx.With(c.Context(), tenantID), user.ID),
		activity.CategoryAuthentication, "signup",
		activity.WithEntity("tenant", tenantID), activity.WithSource(activity.SourceAPI),
	)

	return c.Status(fiber.StatusCreated).JSON(signupResponse{Tenant: tenant, User: user, Session: session})
}

// Current godoc
// @Summary      Get current tenant
// @Description  Returns the authenticated caller's tenant.
// @Tags         tenants
// @Produce      json
// @Security     CookieAuth
// @Success      200  {object}  models.Tenant
// @Failure      401  {object}  map[string]string
// @Router       /api/tenants/current [get]
func (h *TenantsHandler) Current(c *fiber.Ctx) error {
	return h.respondTenant(c, h.callerTenantID(c))
}

// Get godoc
// @Summary      Get tenant
// @Description  Returns a tenant by id. Only the caller's own tenant is visible; any other id returns 404 (no existence leak — CON-97 §12.3).
// @Tags         tenants
// @Produce      json
// @Security     CookieAuth
// @Param        id   path      string  true  "Tenant id"
// @Success      200  {object}  models.Tenant
// @Failure      404  {object}  map[string]string
// @Router       /api/tenants/{id} [get]
func (h *TenantsHandler) Get(c *fiber.Ctx) error {
	if c.Params("id") != h.callerTenantID(c) {
		return fiber.NewError(fiber.StatusNotFound, "tenant not found")
	}
	return h.respondTenant(c, c.Params("id"))
}

type updateTenantRequest struct {
	Name string `json:"name" validate:"required"`
}

// Update godoc
// @Summary      Update tenant
// @Description  Updates the caller's own tenant name. Any other id returns 404. The slug is stable across renames (CON-97 §7.3).
// @Tags         tenants
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        id    path      string               true  "Tenant id"
// @Param        body  body      updateTenantRequest  true  "Tenant payload"
// @Success      200   {object}  models.Tenant
// @Failure      400   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/tenants/{id} [put]
func (h *TenantsHandler) Update(c *fiber.Ctx) error {
	if c.Params("id") != h.callerTenantID(c) {
		return fiber.NewError(fiber.StatusNotFound, "tenant not found")
	}

	var req updateTenantRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	tenant, err := h.tenantRepo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "tenant not found")
		}
		return err
	}

	tenant.Name = req.Name
	tenant.UpdatedAt = time.Now().UTC()
	if err := h.tenantRepo.Update(c.Context(), tenant); err != nil {
		return err
	}
	h.activity.Record(c.Context(), activity.CategoryAuthentication, "tenant_updated",
		activity.WithEntity("tenant", tenant.ID), activity.WithSource(activity.SourceAPI))
	return c.JSON(tenant)
}

// callerTenantID returns the tenant id of the authenticated caller, read from
// the request context (set by RequireAuth via tenantctx.Key), with the session
// local as a defensive fallback.
func (h *TenantsHandler) callerTenantID(c *fiber.Ctx) string {
	if id, ok := tenantctx.From(c.Context()); ok {
		return id
	}
	if s, ok := c.Locals("session").(*models.Session); ok && s != nil {
		return s.TenantID
	}
	return ""
}

func (h *TenantsHandler) respondTenant(c *fiber.Ctx, id string) error {
	tenant, err := h.tenantRepo.GetByID(c.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusNotFound, "tenant not found")
		}
		return err
	}
	return c.JSON(tenant)
}

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slugify produces a lowercase, URL-safe label from a tenant name.
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugNonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "tenant"
	}
	return s
}

// uniqueSlug returns slugify(name), suffixed with -2, -3, … until free.
func (h *TenantsHandler) uniqueSlug(ctx context.Context, name string) (string, error) {
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

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// isUniqueViolationOn reports whether err is a unique-constraint violation on
// the specifically-named constraint. Callers that special-case one constraint
// use this so an unrelated unique conflict follows the normal error path.
func isUniqueViolationOn(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
