package handlers

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/secrets"
)

// SecretsHandler is the REST surface over the SecretStore. Plaintext
// only enters via PUT bodies and never leaves: GET returns metadata
// only, errors never echo the submitted value, and request logging
// records the secret name + status but not the body.
type SecretsHandler struct {
	store secrets.Store
	auth  fiber.Handler
}

func NewSecretsHandler(store secrets.Store, auth fiber.Handler) *SecretsHandler {
	return &SecretsHandler{store: store, auth: auth}
}

func (h *SecretsHandler) Register(app *fiber.App) {
	g := app.Group("/api/secrets")
	g.Get("/", h.auth, h.List)
	g.Get("/:name", h.auth, h.Get)
	g.Put("/:name", h.auth, h.Put)
	g.Delete("/:name", h.auth, h.Delete)
}

type secretPutRequest struct {
	Value string `json:"value"`
}

// List godoc
// @Summary      List configured secrets (metadata only)
// @Description  Returns metadata for every persisted secret. Never
// @Description  returns ciphertext, nonces, wrapped DEKs, or plaintext.
// @Tags         secrets
// @Produce      json
// @Security     CookieAuth
// @Success      200  {array}   secrets.Metadata
// @Failure      401  {object}  map[string]string
// @Router       /api/secrets [get]
func (h *SecretsHandler) List(c *fiber.Ctx) error {
	start := time.Now()
	out, err := h.store.List(c.Context())
	logRequest(c, "", start, err)
	if err != nil {
		return err
	}
	if out == nil {
		out = []secrets.Metadata{}
	}
	return c.JSON(out)
}

// Get godoc
// @Summary      Get secret metadata
// @Description  Returns metadata for a single secret. 404 if absent;
// @Description  the plaintext is never returned.
// @Tags         secrets
// @Produce      json
// @Security     CookieAuth
// @Param        name  path      string  true  "Secret name"
// @Success      200   {object}  secrets.Metadata
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Failure      404   {object}  map[string]string
// @Router       /api/secrets/{name} [get]
func (h *SecretsHandler) Get(c *fiber.Ctx) error {
	start := time.Now()
	name := c.Params("name")
	meta, err := h.store.GetMetadata(c.Context(), name)
	logRequest(c, name, start, err)
	if err != nil {
		return mapSecretError(err)
	}
	return c.JSON(meta)
}

// Put godoc
// @Summary      Upsert a secret
// @Description  Creates or replaces a secret. The plaintext is never
// @Description  echoed back; the response carries metadata only.
// @Tags         secrets
// @Accept       json
// @Produce      json
// @Security     CookieAuth
// @Param        name  path      string             true  "Secret name (allowlisted)"
// @Param        body  body      secretPutRequest   true  "Plaintext value"
// @Success      200   {object}  secrets.Metadata
// @Success      201   {object}  secrets.Metadata
// @Failure      400   {object}  map[string]string
// @Failure      401   {object}  map[string]string
// @Router       /api/secrets/{name} [put]
func (h *SecretsHandler) Put(c *fiber.Ctx) error {
	start := time.Now()
	name := c.Params("name")

	// We parse the body into a typed request rather than ad-hoc map
	// access so the only place value ever lives is `req.Value` —
	// scoped to this function, not retained anywhere.
	var req secretPutRequest
	if err := c.BodyParser(&req); err != nil {
		logRequest(c, name, start, err)
		// The parse error from BodyParser comes from gofiber's JSON
		// decoder and shouldn't contain the raw body. Even so, we
		// return a fixed-shape message to be safe.
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	meta, created, err := h.store.Set(c.Context(), name, req.Value)
	logRequest(c, name, start, err)
	if err != nil {
		return mapSecretError(err)
	}
	status := fiber.StatusOK
	if created {
		status = fiber.StatusCreated
	}
	return c.Status(status).JSON(meta)
}

// Delete godoc
// @Summary      Delete a secret
// @Description  Removes a secret. Features depending on the deleted
// @Description  key degrade on next call (no boot failure). 404 if
// @Description  the name is absent.
// @Tags         secrets
// @Security     CookieAuth
// @Param        name  path  string  true  "Secret name"
// @Success      204
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/secrets/{name} [delete]
func (h *SecretsHandler) Delete(c *fiber.Ctx) error {
	start := time.Now()
	name := c.Params("name")
	err := h.store.Delete(c.Context(), name)
	logRequest(c, name, start, err)
	if err != nil {
		return mapSecretError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mapSecretError translates Store errors to HTTP statuses. Notably
// ErrUnknownName → 400 (the name is fine but isn't on our allowlist),
// ErrNotFound → 404, ErrInvalidValue → 400, anything else → 500.
func mapSecretError(err error) error {
	switch {
	case errors.Is(err, secrets.ErrUnknownName):
		return fiber.NewError(fiber.StatusBadRequest, "unknown secret name")
	case errors.Is(err, secrets.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "secret not found")
	case errors.Is(err, secrets.ErrInvalidValue):
		// We pass the validation message through verbatim because the
		// store guarantees it never contains the rejected value (only
		// length / position metadata).
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return err
	}
}

// logRequest emits a structured-style log line. Includes status,
// duration, secret name (if known), and the error class — never the
// request body, the bearer token, or any value derived from key
// material.
func logRequest(c *fiber.Ctx, name string, start time.Time, err error) {
	dur := time.Since(start)
	status := c.Response().StatusCode()
	if err != nil {
		// Fiber doesn't update the status code until the handler
		// returns; pre-compute what mapSecretError will set so the log
		// line agrees with the response.
		status = errStatus(err)
	}
	if name == "" {
		slog.InfoContext(c.Context(), "secrets request", logging.AttrComponent, "secrets", "method", c.Method(), "path", c.Path(), "status", status, "duration_ms", dur.Milliseconds())
		return
	}
	slog.InfoContext(c.Context(), "secrets request", logging.AttrComponent, "secrets", "method", c.Method(), "path", c.Path(), "name", name, "status", status, "duration_ms", dur.Milliseconds())
}

func errStatus(err error) int {
	if err == nil {
		return fiber.StatusOK
	}
	switch {
	case errors.Is(err, secrets.ErrUnknownName), errors.Is(err, secrets.ErrInvalidValue):
		return fiber.StatusBadRequest
	case errors.Is(err, secrets.ErrNotFound):
		return fiber.StatusNotFound
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return fe.Code
	}
	return fiber.StatusInternalServerError
}
