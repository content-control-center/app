package handlers

import (
	"database/sql"
	"errors"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/content-control-center/app/src/integrations/zernio"
	"github.com/content-control-center/app/src/repository"
)

// connectLinkTTL is the client-side TTL hint returned with each connect
// link. Zernio enforces the actual expiry; this is informational only.
const connectLinkTTL = 30 * time.Minute

// fastPollWindow is the fast-cadence window the worker honours after a
// connect link is issued. Per the ticket: 10 minutes.
const fastPollWindow = 10 * time.Minute

// ZernioHandler exposes the integration endpoints under
// /api/integrations/zernio.
type ZernioHandler struct {
	integ        *zernio.Integration
	bootstrapper *zernio.Bootstrapper
	settings     zernio.SettingsStore
	platforms    repository.PlatformRepository
	accounts     repository.SocialAccountRepository
	worker       *zernio.Worker
	rateLimiter  *zernio.RateLimiter
	auth         fiber.Handler
}

func NewZernioHandler(
	integ *zernio.Integration,
	bootstrapper *zernio.Bootstrapper,
	settings zernio.SettingsStore,
	platforms repository.PlatformRepository,
	accounts repository.SocialAccountRepository,
	worker *zernio.Worker,
	rateLimiter *zernio.RateLimiter,
	auth fiber.Handler,
) *ZernioHandler {
	return &ZernioHandler{
		integ:        integ,
		bootstrapper: bootstrapper,
		settings:     settings,
		platforms:    platforms,
		accounts:     accounts,
		worker:       worker,
		rateLimiter:  rateLimiter,
		auth:         auth,
	}
}

func (h *ZernioHandler) Register(app *fiber.App) {
	// Health is intentionally unauthenticated, matching /api/health —
	// monitoring agents need to scrape it without holding a session.
	app.Get("/api/integrations/zernio/health", h.Health)

	g := app.Group("/api/integrations/zernio", h.auth)
	g.Get("/platforms", h.ListPlatforms)
	g.Post("/connect-links", h.CreateConnectLink)
	g.Get("/accounts", h.ListAccounts)
	g.Post("/sync", h.TriggerSync)
	g.Post("/profile/repair", h.RepairProfile)
}

type healthResponse struct {
	Enabled        bool   `json:"enabled"`
	State          string `json:"state"`
	ProfileID      string `json:"profileId,omitempty"`
	LastSyncAt     string `json:"lastSyncAt,omitempty"`
	LastSyncStatus string `json:"lastSyncStatus,omitempty"`
	AccountCount   int    `json:"accountCount"`
}

// Health godoc
// @Summary      Zernio integration health
// @Description  Public endpoint suitable for inclusion in monitoring
// @Description  dashboards. Reports whether the integration is enabled,
// @Description  its state (disabled / degraded / ok), the bootstrapped
// @Description  profile ID, and the most recent sync result.
// @Tags         zernio
// @Produce      json
// @Success      200  {object}  healthResponse
// @Router       /api/integrations/zernio/health [get]
func (h *ZernioHandler) Health(c *fiber.Ctx) error {
	resp := healthResponse{
		Enabled: h.integ.Enabled(),
		State:   string(h.integ.State()),
	}
	profileID, _, err := h.settings.Get(c.Context(), zernio.SettingProfileID)
	if err != nil {
		return err
	}
	resp.ProfileID = profileID
	if profileID != "" {
		rows, err := h.accounts.ListActive(c.Context(), profileID)
		if err != nil {
			return err
		}
		resp.AccountCount = len(rows)
	}
	resp.LastSyncAt, _, _ = h.settings.Get(c.Context(), zernio.SettingLastSyncAt)
	resp.LastSyncStatus, _, _ = h.settings.Get(c.Context(), zernio.SettingLastSyncStatus)
	return c.JSON(resp)
}

// platformInfo is the GET /platforms entry shape.
type platformInfo struct {
	ID                 string   `json:"id"`
	Label              string   `json:"label"`
	SupportedPostTypes []string `json:"supportedPostTypes"`
}

type platformsResponse struct {
	Platforms []platformInfo `json:"platforms"`
}

// ListPlatforms godoc
// @Summary      List Zernio-supported platforms (Phase 1 allowlist)
// @Description  Returns the Phase 1 allowlist with each platform's
// @Description  supportedPostTypes derived from the local platforms
// @Description  table. Clients render a picker from this list.
// @Tags         zernio
// @Produce      json
// @Security     CookieAuth
// @Success      200  {object}  platformsResponse
// @Failure      401  {object}  map[string]string
// @Router       /api/integrations/zernio/platforms [get]
func (h *ZernioHandler) ListPlatforms(c *fiber.Ctx) error {
	allowlist := zernio.SupportedPlatforms()
	out := make([]platformInfo, 0, len(allowlist))
	for _, p := range allowlist {
		platform, err := h.platforms.GetByID(c.Context(), p.OgenID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		// A missing local platforms row is treated as "no post types known"
		// rather than a failure — the allowlist is the source of truth for
		// what Zernio supports; the post-types map is best-effort metadata.
		var postTypes []string
		if platform != nil {
			postTypes = make([]string, 0, len(platform.PostTypes))
			for slug := range platform.PostTypes {
				postTypes = append(postTypes, slug)
			}
		}
		out = append(out, platformInfo{
			ID:                 p.ZernioID,
			Label:              p.Label,
			SupportedPostTypes: postTypes,
		})
	}
	return c.JSON(platformsResponse{Platforms: out})
}

type connectLinkRequest struct {
	Platform string `json:"platform" validate:"required"`
}

type connectLinkResponse struct {
	Platform   string    `json:"platform"`
	ConnectURL string    `json:"connectUrl"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// CreateConnectLink godoc
// @Summary      Create a Zernio connect link
// @Description  Returns a one-shot connect URL that the user opens in
// @Description  a browser to authorize a social account on the chosen
// @Description  platform. The URL contains a short-lived token.
// @Tags         zernio
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        body  body      connectLinkRequest  true  "Platform to connect"
// @Success      200   {object}  connectLinkResponse
// @Failure      400   {object}  map[string]string  "invalid_platform"
// @Failure      401   {object}  map[string]string
// @Failure      409   {object}  map[string]string  "integration_disabled"
// @Failure      429   {object}  map[string]string  "rate_limited"
// @Failure      503   {object}  map[string]string  "integration_degraded"
// @Router       /api/integrations/zernio/connect-links [post]
func (h *ZernioHandler) CreateConnectLink(c *fiber.Ctx) error {
	if !h.integ.Enabled() {
		return fiber.NewError(fiber.StatusConflict, "integration_disabled")
	}
	if h.integ.State() != zernio.StateOK {
		return fiber.NewError(fiber.StatusServiceUnavailable, "integration_degraded")
	}

	var req connectLinkRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := validate.Struct(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
	}
	if zernio.LookupSupportedPlatform(req.Platform) == nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid_platform")
	}

	if allow, retryAfter := h.rateLimiter.Allow(); !allow {
		seconds := int(math.Ceil(retryAfter.Seconds()))
		if seconds < 1 {
			seconds = 1
		}
		c.Set("Retry-After", strconv.Itoa(seconds))
		return fiber.NewError(fiber.StatusTooManyRequests, "rate_limited")
	}

	profileID, ok, err := h.settings.Get(c.Context(), zernio.SettingProfileID)
	if err != nil {
		return err
	}
	if !ok || profileID == "" {
		// Bootstrap should have populated this in StateOK; treat its
		// absence as a degraded transient and surface as 503.
		return fiber.NewError(fiber.StatusServiceUnavailable, "integration_degraded")
	}

	connectURL, err := h.integ.Client.CreateConnectLink(c.Context(), profileID, req.Platform)
	if err != nil {
		var apiErr *zernio.APIError
		if errors.As(err, &apiErr) {
			return fiber.NewError(http.StatusBadGateway, apiErr.Error())
		}
		return err
	}

	// Adaptive polling window: tighten the worker cadence for the next
	// fastPollWindow so the user sees their connected account fast.
	h.integ.BumpFastUntil(time.Now().Add(fastPollWindow))

	// Log redacted URL only — the query string contains a short-lived
	// token that must not appear in log lines.
	log.Printf("zernio: connect link issued (platform=%s profile=%s url=%s)",
		req.Platform, profileID, redactConnectURL(connectURL))

	return c.JSON(connectLinkResponse{
		Platform:   req.Platform,
		ConnectURL: connectURL,
		ExpiresAt:  time.Now().UTC().Add(connectLinkTTL),
	})
}

type accountsResponse struct {
	Accounts       []accountInfo `json:"accounts"`
	LastSyncAt     string        `json:"lastSyncAt,omitempty"`
	LastSyncStatus string        `json:"lastSyncStatus,omitempty"`
}

type accountInfo struct {
	ID            string    `json:"id"`
	Platform      string    `json:"platform"`
	Username      string    `json:"username"`
	DisplayName   string    `json:"displayName"`
	AvatarURL     string    `json:"avatarUrl"`
	IsActive      bool      `json:"isActive"`
	ConnectedAt   time.Time `json:"connectedAt"`
	LastSyncedAt  time.Time `json:"lastSyncedAt"`
}

// ListAccounts godoc
// @Summary      List Zernio social accounts (local mirror)
// @Description  Returns the local view of accounts attached via the
// @Description  Zernio profile, plus the last sync timestamp + status.
// @Description  Reads from SQLite — does not call Zernio.
// @Tags         zernio
// @Produce      json
// @Security     CookieAuth
// @Success      200  {object}  accountsResponse
// @Failure      401  {object}  map[string]string
// @Failure      409  {object}  map[string]string  "integration_disabled"
// @Router       /api/integrations/zernio/accounts [get]
func (h *ZernioHandler) ListAccounts(c *fiber.Ctx) error {
	if !h.integ.Enabled() {
		return fiber.NewError(fiber.StatusConflict, "integration_disabled")
	}
	profileID, _, err := h.settings.Get(c.Context(), zernio.SettingProfileID)
	if err != nil {
		return err
	}
	var rows []accountInfo
	if profileID != "" {
		fetched, err := h.accounts.ListActive(c.Context(), profileID)
		if err != nil {
			return err
		}
		rows = make([]accountInfo, 0, len(fetched))
		for _, a := range fetched {
			rows = append(rows, accountInfo{
				ID:           a.ID,
				Platform:     a.Platform,
				Username:     a.Username,
				DisplayName:  a.DisplayName,
				AvatarURL:    a.AvatarURL,
				IsActive:     a.IsActive,
				ConnectedAt:  a.ConnectedAt,
				LastSyncedAt: a.LastSyncedAt,
			})
		}
	}
	lastAt, _, _ := h.settings.Get(c.Context(), zernio.SettingLastSyncAt)
	lastStatus, _, _ := h.settings.Get(c.Context(), zernio.SettingLastSyncStatus)
	return c.JSON(accountsResponse{
		Accounts:       rows,
		LastSyncAt:     lastAt,
		LastSyncStatus: lastStatus,
	})
}

// TriggerSync godoc
// @Summary      Trigger a Zernio sync tick
// @Description  Asks the background worker to run a sync at its
// @Description  earliest opportunity. Returns 202 with the previous
// @Description  last_sync_at value so callers can poll for progress.
// @Tags         zernio
// @Produce      json
// @Security     CookieAuth
// @Success      202  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      409  {object}  map[string]string  "integration_disabled"
// @Router       /api/integrations/zernio/sync [post]
func (h *ZernioHandler) TriggerSync(c *fiber.Ctx) error {
	if !h.integ.Enabled() {
		return fiber.NewError(fiber.StatusConflict, "integration_disabled")
	}
	prevAt, _, _ := h.settings.Get(c.Context(), zernio.SettingLastSyncAt)
	if h.worker != nil {
		h.worker.TriggerNow()
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"previousLastSyncAt": prevAt,
	})
}

// RepairProfile godoc
// @Summary      Repair Zernio profile
// @Description  Triggers a fresh Zernio profile bootstrap for operational
// @Description  recovery. Idempotent: a successful repair leaves the
// @Description  integration in the "ok" state.
// @Tags         zernio
// @Produce      json
// @Security     CookieAuth
// @Success      202  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      409  {object}  map[string]string  "integration_disabled"
// @Failure      500  {object}  map[string]string
// @Router       /api/integrations/zernio/profile/repair [post]
func (h *ZernioHandler) RepairProfile(c *fiber.Ctx) error {
	if !h.integ.Enabled() {
		return fiber.NewError(fiber.StatusConflict, "integration_disabled")
	}
	if err := h.bootstrapper.Run(c.Context()); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"state": string(h.integ.State()),
	})
}

// redactConnectURL strips the query string from a connect URL so log
// lines record host + path without the short-lived token.
func redactConnectURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return "<invalid url>"
	}
	return u.Scheme + "://" + u.Host + u.Path
}
