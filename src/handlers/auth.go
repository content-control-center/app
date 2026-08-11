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
//
// Since CON-147 the active workspace is resolved PER REQUEST, not bound to the
// session: an X-Workspace-Id header selects which of the account's workspaces
// this request acts in (the session's stored tenant is only the default that
// seeds a fresh tab). This is what lets two browser tabs, sharing one session
// cookie, operate in two different workspaces at once. The session cookie
// identifies the account; the header identifies the workspace.
//
// The resolution rewrites the in-request session's UserID/TenantID to the active
// membership/workspace before handlers run, so every downstream reader —
// tenantctx scoping, callerUser/requireOwner, and the ~20 handlers that stamp
// session.UserID as CreatedBy — sees the active workspace with no per-handler
// change. The stored session row is untouched (its tenant is the default, moved
// only by POST /api/workspaces/:id/switch).
func RequireAuth(sessionRepo repository.SessionRepository, userRepo repository.UserRepository, cookieName string) fiber.Handler {
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

		// The session's stored tenant is the DEFAULT workspace — where a fresh tab
		// or the next login seeds. Capture it before the active-workspace override
		// below rewrites session.TenantID, so the switcher can mark it (CON-147).
		c.Locals(DefaultWorkspaceLocal, session.TenantID)

		// Resolve the active workspace. Absent header (or one naming the default)
		// keeps the session's stored membership — the common single-workspace path,
		// with no extra query. A header naming another workspace is validated
		// against membership and, on success, rewrites the in-request session view
		// to that workspace; a non-member is rejected (403) so a request can never
		// read a workspace the account isn't in.
		if ws := c.Get(workspaceHeader); ws != "" && ws != session.TenantID {
			membership, merr := userRepo.GetMembership(c.Context(), session.AccountID, ws)
			if merr != nil {
				if errors.Is(merr, sql.ErrNoRows) {
					return fiber.NewError(fiber.StatusForbidden, "not a member of this workspace")
				}
				return merr
			}
			session.UserID = membership.ID
			session.TenantID = ws
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

// workspaceHeader is the per-request active-workspace selector (CON-147). A
// request carrying it acts in that workspace if the account is a member; absent,
// the session's default workspace applies.
const workspaceHeader = "X-Workspace-Id"

// DefaultWorkspaceLocal is the c.Locals key under which RequireAuth stashes the
// session's stored default workspace (before the active-workspace override), so
// the workspace switcher can flag which row is the default.
const DefaultWorkspaceLocal = "default_workspace_id"

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

// requireOwner loads the caller and returns it only if they are an owner of the
// active workspace, otherwise a 403 (CON-26 §7). The team-management endpoints
// (invitations, member role changes and removals) gate on this. Enforcement
// reads the live role from the caller's membership of the active workspace
// (RequireAuth set session.UserID to it), so the same account can be an owner of
// one workspace and a member of another, and a role change takes effect
// immediately with no session-schema change.
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
