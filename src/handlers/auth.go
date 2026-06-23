package handlers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// RequireAuth returns a middleware that rejects requests without a valid,
// non-expired session cookie. On success it stores the *models.Session under
// the "session" local key for downstream handlers.
func RequireAuth(sessionRepo repository.SessionRepository, cookieName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Cookies(cookieName)
		if token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}

		session, err := sessionRepo.GetByID(c.Context(), token)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired session")
			}
			return err
		}

		if time.Now().UTC().After(session.ExpiresAt) {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired session")
		}

		c.Locals("session", session)
		// Carry the tenant so the tenant-scoped query layer (CON-97 §6) can
		// read it via tenantctx.From(c.Context()) without threading the
		// context through every handler. fasthttp exposes c.Locals values
		// through (*RequestCtx).Value, so the same key reads back as a
		// context value.
		c.Locals(tenantctx.Key, session.TenantID)
		return c.Next()
	}
}
