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

// Account mirrors a Zernio social-account object. Known fields are
// typed; the full payload is preserved in Raw so future Zernio fields
// survive into the local raw_json column unchanged.
type Account struct {
	ID          string          `json:"_id"`
	ProfileID   string          `json:"profileId"`
	Platform    string          `json:"platform"`
	Username    string          `json:"username"`
	DisplayName string          `json:"displayName"`
	AvatarURL   string          `json:"avatarUrl"`
	IsActive    bool            `json:"isActive"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	Raw         json.RawMessage `json:"-"`
}

// ListAccounts returns every account attached to the given profile.
// Zernio paginates; the worker's reconciliation needs the full set so
// we walk pages until the response returns fewer items than limit.
func (c *Client) ListAccounts(ctx context.Context, profileID string) ([]Account, error) {
	if c == nil {
		return nil, errors.New("zernio: client is disabled")
	}
	const pageSize = 100
	var out []Account
	for offset := 0; ; offset += pageSize {
		var env struct {
			Items []json.RawMessage `json:"items"`
		}
		q := url.Values{
			"profileId": {profileID},
			"limit":     {strconv.Itoa(pageSize)},
			"offset":    {strconv.Itoa(offset)},
		}
		if err := c.do(ctx, http.MethodGet, "/accounts", q, nil, &env); err != nil {
			return nil, err
		}
		for _, raw := range env.Items {
			a, err := decodeAccount(raw)
			if err != nil {
				return nil, err
			}
			out = append(out, *a)
		}
		if len(env.Items) < pageSize {
			break
		}
	}
	return out, nil
}

func decodeAccount(raw json.RawMessage) (*Account, error) {
	var a Account
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("zernio: decode account: %w", err)
	}
	a.Raw = append(json.RawMessage(nil), raw...)
	return &a, nil
}
