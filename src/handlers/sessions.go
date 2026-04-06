package handlers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/models"
)

const sessionTTL = 7 * 24 * time.Hour

type SessionsHandler struct {
	db         *bun.DB
	cookieName string
}

func NewSessionsHandler(db *bun.DB, cookieName string) *SessionsHandler {
	return &SessionsHandler{db: db, cookieName: cookieName}
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

	user := new(models.User)
	err := h.db.NewSelect().Model(user).Where("u.email = ?", req.Email).Scan(c.Context())
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
		ExpiresAt: time.Now().UTC().Add(sessionTTL),
	}
	if _, err := h.db.NewInsert().Model(session).Exec(c.Context()); err != nil {
		return err
	}

	c.Cookie(&fiber.Cookie{
		Name:     h.cookieName,
		Value:    token,
		Expires:  session.ExpiresAt,
		HTTPOnly: true,
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

	res, err := h.db.NewDelete().Model((*models.Session)(nil)).Where("id = ?", token).Exec(c.Context())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid session")
	}

	c.Cookie(&fiber.Cookie{
		Name:    h.cookieName,
		Value:   "",
		Expires: time.Unix(0, 0),
		MaxAge:  -1,
	})

	return c.SendStatus(fiber.StatusNoContent)
}
