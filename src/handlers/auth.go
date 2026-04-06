package handlers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/content-control-center/app/src/repository"
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
		return c.Next()
	}
}
