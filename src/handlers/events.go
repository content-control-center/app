package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/kernel/logging"
	"github.com/ogen-app/ogen/src/kernel/tenantctx"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// defaultHeartbeatInterval keeps idle SSE connections alive past
// load-balancer timeouts (typically 30–60s) and gives us a write probe
// to detect dead clients via send failures.
const defaultHeartbeatInterval = 20 * time.Second

// EventsHandler streams Hub events to authenticated clients over SSE.
type EventsHandler struct {
	hub               eventhub.Hub
	sessionRepo       repository.SessionRepository
	auth              fiber.Handler
	heartbeatInterval time.Duration
}

// NewEventsHandler wires the SSE stream endpoint. heartbeatInterval = 0
// uses the package default (20s); tests pass a smaller value.
func NewEventsHandler(
	hub eventhub.Hub,
	sessionRepo repository.SessionRepository,
	auth fiber.Handler,
	heartbeatInterval time.Duration,
) *EventsHandler {
	if heartbeatInterval <= 0 {
		heartbeatInterval = defaultHeartbeatInterval
	}
	return &EventsHandler{
		hub:               hub,
		sessionRepo:       sessionRepo,
		auth:              auth,
		heartbeatInterval: heartbeatInterval,
	}
}

func (h *EventsHandler) Register(app *fiber.App) {
	app.Get("/api/events", h.auth, h.Stream)
}

// Stream godoc
// @Summary      Event stream (SSE)
// @Description  Subscribes the authenticated user to a long-lived
// @Description  Server-Sent Events stream filtered by topic patterns.
// @Description
// @Description  Topic filters are passed via the `topics` query param as a
// @Description  comma-separated list. Supported forms per filter:
// @Description    - "all"               — receive every event the user is
// @Description                            authorised to see.
// @Description    - "kind:id"           — exact match (e.g. "entity:post:Xq").
// @Description    - "kind:*"            — prefix wildcard (e.g. "job:*").
// @Description
// @Description  Each event is emitted as one SSE frame:
// @Description    id: <event-id>
// @Description    event: <type>
// @Description    data: <json>
// @Description
// @Description  A heartbeat comment (`: ping`) is sent every 20 seconds.
// @Description  Delivery is at-most-once; the server holds no event log.
// @Description  Subscribers are disconnected if their per-connection buffer
// @Description  overflows (the client should reconnect and reconcile via REST).
// @Description
// @Description  The `Last-Event-ID` request header is currently accepted
// @Description  and ignored — reserved for future replay support.
// @Tags         events
// @Produce      text/event-stream
// @Security     CookieAuth
// @Param        topics  query  string  true  "Comma-separated topic filters"
// @Success      200  "SSE stream"
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      429  {object}  map[string]string
// @Router       /api/events [get]
func (h *EventsHandler) Stream(c *fiber.Ctx) error {
	session, ok := c.Locals("session").(*models.Session)
	if !ok || session == nil {
		// auth middleware should have caught this, but defend in depth
		return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
	}

	topics, err := parseTopicsParam(c.Query("topics"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	// Last-Event-ID is parsed but currently unused (forward-compat hook
	// per the AC). Documented in swagger above.
	_ = c.Get("Last-Event-Id")

	eventCh, unsubscribe, err := h.hub.Subscribe(c.Context(), eventhub.SubscribeOpts{
		UserID:   session.UserID,
		TenantID: session.TenantID, // CON-97 §10.2: only this tenant's events
		Topics:   topics,
	})
	if err != nil {
		if errors.Is(err, eventhub.ErrTooManySubscribers) {
			return fiber.NewError(fiber.StatusTooManyRequests, err.Error())
		}
		if errors.Is(err, eventhub.ErrNoTopics) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return err
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")

	// Capture into locals before the stream writer runs — the fiber ctx
	// is cancelled the moment this handler returns, so the writer goroutine
	// must not depend on it.
	sessionID := session.ID
	sessionRepo := h.sessionRepo
	heartbeat := h.heartbeatInterval

	// The stream writer runs on a fasthttp goroutine after this handler
	// returns, at which point c.Context() (the *fasthttp.RequestCtx) has been
	// reset and returned to the pool — reading it then is a use-after-free
	// that nil-panics inside the slog ContextHandler, on a goroutine the Fiber
	// recover middleware can't see, taking the whole process down (CON-158).
	// Detach a logging context now so writer-side logs still correlate with
	// request_id / tenant_id / user_id without touching the recycled ctx.
	reqID, _ := logging.RequestIDFrom(c.Context())
	logCtx := logging.WithRequestID(context.Background(), reqID)
	logCtx = logging.WithUserID(logCtx, session.UserID)
	logCtx = tenantctx.With(logCtx, session.TenantID)

	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		defer unsubscribe()

		// Send an initial heartbeat so the client confirms a working
		// connection before any real event arrives.
		if err := writeHeartbeat(w); err != nil {
			return
		}

		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()

		for {
			select {
			case ev, ok := <-eventCh:
				if !ok {
					// Hub disconnected us (backpressure or shutdown).
					return
				}
				if err := writeSSEEvent(w, ev); err != nil {
					slog.ErrorContext(logCtx, "sse write failed", logging.AttrComponent, "events", logging.AttrError, err)
					return
				}
			case <-ticker.C:
				if err := writeHeartbeat(w); err != nil {
					return
				}
				// Periodic session recheck. If the session was invalidated
				// (logout, expiry, deletion) we drop the stream now rather
				// than keep delivering events to an unauthenticated client.
				if !sessionStillValid(sessionRepo, sessionID) {
					slog.InfoContext(logCtx, "session no longer valid; closing stream", logging.AttrComponent, "events")
					return
				}
			}
		}
	}))

	return nil
}

// parseTopicsParam splits and trims a comma-separated topics query value.
// Returns ErrNoTopics if no usable topic remains after trimming.
func parseTopicsParam(raw string) ([]string, error) {
	if raw == "" {
		return nil, errors.New("topics query param is required (e.g. ?topics=all or ?topics=job:*,entity:post:abc)")
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("topics query param must contain at least one non-empty topic")
	}
	return out, nil
}

// writeSSEEvent emits one well-formed SSE frame. JSON is written on a
// single `data:` line — the events the Hub carries are flat enough that
// multi-line data isn't needed today.
func writeSSEEvent(w *bufio.Writer, ev eventhub.Event) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	eventType := ev.Type
	if eventType == "" {
		eventType = "message" // SSE default; helps EventSource consumers
	}
	if _, err := fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", ev.ID, eventType, body); err != nil {
		return err
	}
	return w.Flush()
}

// writeHeartbeat sends a comment frame. SSE comments start with `:` and
// are ignored by clients but keep proxies/load balancers from killing
// the idle connection.
func writeHeartbeat(w *bufio.Writer) error {
	if _, err := w.WriteString(": ping\n\n"); err != nil {
		return err
	}
	return w.Flush()
}

// sessionStillValid runs a fresh repo lookup. Background context — the
// fiber request ctx is gone by the time the stream writer runs.
func sessionStillValid(repo repository.SessionRepository, id string) bool {
	s, err := repo.GetByID(context.Background(), id)
	if err != nil || s == nil {
		return false
	}
	return time.Now().UTC().Before(s.ExpiresAt)
}
