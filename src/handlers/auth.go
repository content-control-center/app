package handlers

import (
	"database/sql"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/models"
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
		// Carry the user id the same way (CON-107) so the slog ContextHandler
		// attaches user_id to every log line made with c.Context().
		c.Locals(logging.UserIDKey, session.UserID)
		return c.Next()
	}
}

// callerUser loads the authenticated caller's user row (with its role) from the
// session RequireAuth stashed in c.Locals. It is the basis for the CON-26
// authorization checks — owner gating and the self-or-owner rules. The lookup is
// tenant-scoped to the session's tenant (set by RequireAuth), so it resolves the
// caller's own membership. Returns 401 when there is no session or the user is
// gone.
func callerUser(c *fiber.Ctx, userRepo repository.UserRepository) (*models.User, error) {
	session, ok := c.Locals("session").(*models.Session)
	if !ok || session == nil {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}
	user, err := userRepo.GetByID(c.Context(), session.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fiber.NewError(fiber.StatusUnauthorized, "user not found")
		}
		return nil, err
	}
	return user, nil
}

// requireOwner loads the caller and returns it only if they are an owner of
// their workspace, otherwise a 403 (CON-26 §7). The team-management endpoints
// (invitations, member role changes and removals) gate on this. Enforcement
// reads the live role from users rather than a denormalized session field, so a
// role change takes effect immediately with no session-schema change (CON-147
// adds the denormalized session role).
func requireOwner(c *fiber.Ctx, userRepo repository.UserRepository) (*models.User, error) {
	user, err := callerUser(c, userRepo)
	if err != nil {
		return nil, err
	}
	if user.Role != models.RoleOwner {
		return nil, fiber.NewError(fiber.StatusForbidden, "owner role required")
	}
	return user, nil
}
