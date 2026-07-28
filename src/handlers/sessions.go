package handlers

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ogen-app/ogen/src/activity"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/tenantctx"
)

const sessionTTL = 7 * 24 * time.Hour

type SessionsHandler struct {
	userRepo     repository.UserRepository
	sessionRepo  repository.SessionRepository
	cookieName   string
	secureCookie bool
	// activity records CON-125 authentication events (login, logout). Both run
	// outside tenant scope, so the handler builds an explicit tenant+user
	// context per event. nil is a no-op. Wired via SetActivityRecorder.
	activity *activity.Recorder
}

func NewSessionsHandler(userRepo repository.UserRepository, sessionRepo repository.SessionRepository, cookieName string, secureCookie bool) *SessionsHandler {
	return &SessionsHandler{
		userRepo:     userRepo,
		sessionRepo:  sessionRepo,
		cookieName:   cookieName,
		secureCookie: secureCookie,
	}
}

// SetActivityRecorder wires the CON-125 activity recorder (nil-safe no-op).
func (h *SessionsHandler) SetActivityRecorder(r *activity.Recorder) { h.activity = r }

// authCtx builds a context carrying the given tenant + user so the recorder can
// attribute an auth event that happens before the auth middleware would run.
func (h *SessionsHandler) authCtx(c *fiber.Ctx, tenantID, userID string) context.Context {
	return logging.WithUserID(tenantctx.With(c.Context(), tenantID), userID)
}

func (h *SessionsHandler) Register(app *fiber.App) {
	g := app.Group("/api/sessions")
	g.Post("/", h.Create)
	g.Delete("/", h.Delete)
}

type createSessionRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// Create godoc
// @Summary      Login
// @Description  Authenticates a user by email and password, sets a session cookie, and returns the created session.
// @Tags         sessions
// @Accept       json
// @Produce      json
// @Param        body  body      createSessionRequest  true  "Credentials"
// @Success      201   {object}  models.Session
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Router       /api/sessions [post]
func (h *SessionsHandler) Create(c *fiber.Ctx) error {
	var req createSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}

	user, err := h.userRepo.GetByEmail(c.Context(), req.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
		}
		return err
	}

	ok, err := models.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}

	token, err := models.NewSessionToken()
	if err != nil {
		return err
	}

	session := &models.Session{
		ID:        token,
		UserID:    user.ID,
		TenantID:  user.TenantID,
		ExpiresAt: time.Now().UTC().Add(sessionTTL),
	}
	if err := h.sessionRepo.Create(c.Context(), session); err != nil {
		return err
	}

	h.activity.Record(h.authCtx(c, session.TenantID, user.ID), activity.CategoryAuthentication, "login",
		activity.WithEntity("user", user.ID), activity.WithSource(activity.SourceAPI))

	c.Cookie(&fiber.Cookie{
		Name:     h.cookieName,
		Value:    token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HTTPOnly: true,
		Secure:   h.secureCookie,
		SameSite: "Lax",
	})

	return c.Status(fiber.StatusCreated).JSON(session)
}

// Delete godoc
// @Summary      Logout
// @Description  Invalidates the current session and clears the session cookie.
// @Tags         sessions
// @Security     CookieAuth
// @Success      204
// @Failure      401  {object}  map[string]string
// @Router       /api/sessions [delete]
func (h *SessionsHandler) Delete(c *fiber.Ctx) error {
	token := c.Cookies(h.cookieName)
	if token == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "no session")
	}

	// Resolve the session's actor before deleting it, so the logout activity can
	// be attributed to the right tenant/user (this route has no auth middleware,
	// so the request context carries neither). Best-effort — a lookup miss just
	// skips attribution.
	var actorTenant, actorUser string
	if s, gerr := h.sessionRepo.GetByID(c.Context(), token); gerr == nil && s != nil {
		actorTenant, actorUser = s.TenantID, s.UserID
	}

	deleted, err := h.sessionRepo.Delete(c.Context(), token)
	if err != nil {
		return err
	}
	if !deleted {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid session")
	}

	if actorTenant != "" {
		h.activity.Record(h.authCtx(c, actorTenant, actorUser), activity.CategoryAuthentication, "logout",
			activity.WithEntity("user", actorUser), activity.WithSource(activity.SourceAPI))
	}

	// Mirror the attributes used at login so the browser actually replaces the cookie.
	c.Cookie(&fiber.Cookie{
		Name:     h.cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HTTPOnly: true,
		Secure:   h.secureCookie,
		SameSite: "Lax",
	})

	return c.SendStatus(fiber.StatusNoContent)
}
