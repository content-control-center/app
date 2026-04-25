package zernio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Profile mirrors a Zernio profile object. Known fields are typed for
// ergonomic access; the full payload is preserved in Raw so we don't
// silently drop fields when Zernio adds new ones.
type Profile struct {
	ID          string          `json:"_id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Color       string          `json:"color,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	Raw         json.RawMessage `json:"-"`
}

// listProfilesEnvelope is the paged response shape Zernio returns for
// list endpoints. We only consume Items today — Total/Pagination are
// kept on the wire side for forward compatibility.
type listProfilesEnvelope struct {
	Items []json.RawMessage `json:"items"`
}

// ListProfiles returns up to limit profiles. Zernio returns at most
// 100 per page; for the bootstrap we only need to identify a profile
// by name so a single page is sufficient.
func (c *Client) ListProfiles(ctx context.Context, limit int) ([]Profile, error) {
	if c == nil {
		return nil, errors.New("zernio: client is disabled")
	}
	if limit <= 0 {
		limit = 100
	}
	var env listProfilesEnvelope
	q := url.Values{"limit": {strconv.Itoa(limit)}}
	if err := c.do(ctx, http.MethodGet, "/profiles", q, nil, &env); err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(env.Items))
	for _, raw := range env.Items {
		p, err := decodeProfile(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

// GetProfile fetches one profile by ID. Returns an *APIError with
// Status=404 when the profile does not exist on Zernio.
func (c *Client) GetProfile(ctx context.Context, id string) (*Profile, error) {
	if c == nil {
		return nil, errors.New("zernio: client is disabled")
	}
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/profiles/"+url.PathEscape(id), nil, nil, &raw); err != nil {
		return nil, err
	}
	return decodeProfile(raw)
}

// CreateProfile creates a new profile. Zernio returns the created
// object (with a fresh _id and createdAt) which we adopt directly.
func (c *Client) CreateProfile(ctx context.Context, name, description string) (*Profile, error) {
	if c == nil {
		return nil, errors.New("zernio: client is disabled")
	}
	body := map[string]string{"name": name, "description": description}
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodPost, "/profiles", nil, body, &raw); err != nil {
		return nil, err
	}
	return decodeProfile(raw)
}

func decodeProfile(raw json.RawMessage) (*Profile, error) {
	var p Profile
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("zernio: decode profile: %w", err)
	}
	// Defensive copy — the raw slice may share backing storage with the
	// HTTP response buffer, which is reused once the body is closed.
	p.Raw = append(json.RawMessage(nil), raw...)
	return &p, nil
}
