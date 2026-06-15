package zernio

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// sampleAnalyticsItem is the documented per-post response object shape
// (CON-93 §2), returned inside the list envelope's `analytics` array.
const sampleAnalyticsItem = `{
  "postId": "665f-abc",
  "latePostId": "late-665f",
  "status": "published",
  "content": "hello world",
  "publishedAt": "2026-06-15T09:00:00Z",
  "platform": "linkedin",
  "isExternal": false,
  "syncStatus": "synced",
  "analytics": {
    "impressions": 1234, "reach": 1000, "likes": 56, "comments": 7,
    "shares": 3, "saves": 5, "clicks": 12, "views": 800,
    "engagementRate": 0.07, "lastUpdated": "2026-06-15T09:00:00Z"
  },
  "platformAnalytics": [
    {
      "platform": "linkedin", "status": "published",
      "platformPostId": "urn:li:share:1", "accountId": "acc-1",
      "accountUsername": "ogen", "platformPostUrl": "https://li/1",
      "syncStatus": "synced",
      "analytics": { "impressions": 1234, "likes": 56 }
    },
    {
      "platform": "instagram", "status": "published",
      "syncStatus": "unavailable",
      "errorMessage": "analytics scope not granted",
      "reauthorizeUrl": "https://zernio.com/reauth/ig",
      "analytics": {}
    }
  ]
}`

func TestListAnalyticsHappyPath(t *testing.T) {
	s := newStub()
	defer s.Close()

	s.handle("GET", "/analytics", func(w http.ResponseWriter, r *http.Request) {
		// Documented params are forwarded verbatim.
		q := r.URL.Query()
		if q.Get("source") != "late" {
			t.Errorf("source: got %q want late", q.Get("source"))
		}
		if q.Get("limit") != "100" {
			t.Errorf("limit: got %q want 100", q.Get("limit"))
		}
		if q.Get("page") != "2" {
			t.Errorf("page: got %q want 2", q.Get("page"))
		}
		if q.Get("fromDate") != "2026-03-01" {
			t.Errorf("fromDate: got %q want 2026-03-01", q.Get("fromDate"))
		}
		if got := r.Header.Get("Authorization"); got != "Bearer key" {
			t.Errorf("auth header: got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"analytics":[` + sampleAnalyticsItem + `],"pagination":{"page":2,"limit":100,"total":150,"pages":2}}`))
	})

	c := newClient(s)
	items, page, err := c.ListAnalytics(context.Background(), AnalyticsQuery{
		Source:   AnalyticsSourceLate,
		FromDate: "2026-03-01",
		Limit:    100,
		Page:     2,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items: got %d want 1", len(items))
	}
	it := items[0]
	if it.PostID != "665f-abc" || it.LatePostID != "late-665f" {
		t.Errorf("ids: got postId=%q latePostId=%q", it.PostID, it.LatePostID)
	}
	if it.Analytics.Impressions != 1234 || it.Analytics.EngagementRate != 0.07 {
		t.Errorf("metrics: got impressions=%d rate=%v", it.Analytics.Impressions, it.Analytics.EngagementRate)
	}
	if it.Analytics.LastUpdated == nil {
		t.Errorf("lastUpdated should decode")
	}
	if len(it.PlatformAnalytics) != 2 {
		t.Fatalf("platformAnalytics: got %d want 2", len(it.PlatformAnalytics))
	}
	if it.PlatformAnalytics[1].SyncStatus != "unavailable" || it.PlatformAnalytics[1].ErrorMessage == "" {
		t.Errorf("per-platform scope gap not surfaced: %+v", it.PlatformAnalytics[1])
	}
	if len(it.Raw) == 0 {
		t.Errorf("Raw should retain the verbatim item")
	}
	if page.Total != 150 || page.Pages != 2 {
		t.Errorf("pagination: got %+v", page)
	}
}

func TestListAnalyticsLegacy402MapsToSentinel(t *testing.T) {
	s := newStub()
	defer s.Close()
	s.handle("GET", "/analytics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "analytics add-on required"})
	})

	c := newClient(s)
	_, _, err := c.ListAnalytics(context.Background(), AnalyticsQuery{Source: AnalyticsSourceLate})
	if !errors.Is(err, ErrAnalyticsUnavailable) {
		t.Fatalf("err: got %v want ErrAnalyticsUnavailable", err)
	}
}

func TestListAnalyticsPropagatesStatusErrors(t *testing.T) {
	s := newStub()
	defer s.Close()
	s.handle("GET", "/analytics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "slow down"})
	})

	c := newClient(s)
	_, _, err := c.ListAnalytics(context.Background(), AnalyticsQuery{Source: AnalyticsSourceLate})
	if !IsStatus(err, http.StatusTooManyRequests) {
		t.Fatalf("err: got %v want 429 APIError", err)
	}
	// The bearer key must never leak into the error.
	if strings.Contains(err.Error(), "key") {
		t.Errorf("error leaked credential material: %v", err)
	}
}

func TestListAnalyticsNilClientDisabled(t *testing.T) {
	var c *Client
	_, _, err := c.ListAnalytics(context.Background(), AnalyticsQuery{})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("nil client should be disabled, got %v", err)
	}
}
