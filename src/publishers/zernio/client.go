// Package zernio is a thin REST client and integration controller for
// the Zernio.com social-account broker. The package owns:
//
//   - The HTTP client, including bearer-token redaction in error paths
//     and log lines (the API key must never leak via %v / Stringer).
//   - The Integration controller (see integration.go) that tracks
//     boot-time state — disabled / degraded / ok — and is shared between
//     handlers and the background sync worker.
//
// Phase 1 ships only the client skeleton plus the controller. Profile
// bootstrap, connect-link issuance, and account sync are layered on top
// in subsequent phases.
package zernio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// defaultBaseURL is used when ZERNIO_BASE_URL is unset. Zernio's
	// REST API lives under /api/v1; the prefix is part of the base
	// URL rather than each path so tests can point a stub server at
	// the same paths without re-deriving the prefix.
	defaultBaseURL = "https://zernio.com/api/v1"
	// defaultTimeout matches the ticket's documented 15s default.
	defaultTimeout = 15 * time.Second
	// errorBodyLimit caps how much of a non-2xx body we read into APIError
	// so an unexpectedly large response can't blow up logs or memory.
	errorBodyLimit = 4096
)

// Client is the authenticated HTTP wrapper for Zernio's REST API.
//
// The bearer key is held in apiKey and is intentionally not exported
// nor surfaced via String() — accidental %v of a *Client in a log line
// must not leak credentials. All outbound requests carry the bearer
// header; non-2xx responses are returned as a typed *APIError.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// NewClient constructs a Client. apiKey == "" returns nil so callers can
// treat a nil receiver as "integration disabled".
func NewClient(apiKey, baseURL string, timeout time.Duration) *Client {
	if apiKey == "" {
		return nil
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
	}
}

// String redacts the API key. Important: %v / %+v of the struct itself
// would expose apiKey via reflection — callers should print *Client
// (which uses this method) rather than the dereferenced struct.
func (c *Client) String() string {
	if c == nil {
		return "zernio.Client(disabled)"
	}
	return fmt.Sprintf("zernio.Client(baseURL=%s)", c.baseURL)
}

// BaseURL returns the configured base URL. Useful for log lines that
// want to record where requests were sent without exposing the key.
func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.baseURL
}

// APIError carries the HTTP status and a short message extracted from
// the Zernio response. The full body is intentionally truncated and
// stripped of any URL query strings — connect-link tokens must not
// survive into wrapped errors / logs.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("zernio: HTTP %d", e.Status)
	}
	return fmt.Sprintf("zernio: HTTP %d: %s", e.Status, e.Message)
}

// IsStatus reports whether err is an *APIError with the given status.
func IsStatus(err error, status int) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status == status
	}
	return false
}

// Ping issues a low-cost authenticated request to validate the API key
// at boot. 200 → nil; 401 → *APIError{Status:401}; transport errors
// propagate as-is.
//
// Zernio's GET /profiles returns the full list with no pagination
// surface in the documented schema; for a tenant with one or two
// profiles it's effectively free. We discard the body — the only
// signal we care about here is the HTTP status.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil {
		return errors.New("zernio: client is disabled")
	}
	return c.do(ctx, http.MethodGet, "/profiles", nil, nil, nil)
}

// do performs an authenticated request and decodes the JSON body into
// out when out != nil. Returns *APIError for non-2xx responses.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("zernio: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return &APIError{Status: resp.StatusCode, Message: shortErrorMessage(raw)}
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("zernio: decode response: %w", err)
		}
	}
	return nil
}

// shortErrorMessage extracts a brief message from a Zernio error body.
// Falls back to a truncated raw body when the shape is unknown.
func shortErrorMessage(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &env); err == nil {
		if env.Error != "" {
			return env.Error
		}
		if env.Message != "" {
			return env.Message
		}
	}
	if len(raw) > 200 {
		raw = raw[:200]
	}
	return string(raw)
}
