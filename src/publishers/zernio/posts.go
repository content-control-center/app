package zernio

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// JobStatus is Zernio's post-level status enum. The webhook-only
// states (cancelled, recycled) are listed for completeness; this
// integration is polling-only and does not see them.
type JobStatus string

const (
	JobStatusDraft     JobStatus = "draft"
	JobStatusScheduled JobStatus = "scheduled"
	JobStatusPublished JobStatus = "published"
	JobStatusFailed    JobStatus = "failed"
	JobStatusPartial   JobStatus = "partial"
)

// IsTerminal reports whether s indicates the job will not progress on
// its own — the publisher loop should stop polling and resolve the
// owning Post accordingly.
func (s JobStatus) IsTerminal() bool {
	switch s {
	case JobStatusPublished, JobStatusFailed, JobStatusPartial:
		return true
	}
	return false
}

// PlatformVariant is one (platform, account) pairing in a submit
// request. AccountID is the connected social account on Zernio's side.
type PlatformVariant struct {
	Platform  string `json:"platform"`
	AccountID string `json:"accountId"`
}

// PlatformOutcome is one row of the per-platform status returned in
// any post envelope. ErrorMessage is populated only when Status is
// failed for that platform; PlatformPostURL appears once published.
type PlatformOutcome struct {
	Platform        string `json:"platform"`
	AccountID       string `json:"accountId"`
	Status          string `json:"status"`
	ErrorMessage    string `json:"error,omitempty"`
	PlatformPostURL string `json:"platformPostUrl,omitempty"`
}

// SubmitRequest is the body for POST /api/v1/posts. Either ScheduledFor
// (with TZ) or PublishNow must be set; we always go through ScheduledFor
// because the queue dispatch path computes the timestamp explicitly.
type SubmitRequest struct {
	Content      string            `json:"content"`
	Platforms    []PlatformVariant `json:"platforms"`
	ScheduledFor time.Time         `json:"scheduledFor,omitempty"`
	Timezone     string            `json:"timezone,omitempty"`
	PublishNow   bool              `json:"publishNow,omitempty"`
	IsDraft      bool              `json:"isDraft,omitempty"`
	// MediaItems carries opaque media descriptors (URLs to S3 objects,
	// as Zernio expects). Populated by the queue handler from the Post's
	// PostAttachment rows.
	MediaItems []map[string]any `json:"mediaItems,omitempty"`
}

// PostEnvelope mirrors Zernio's standard `{post: {...}}` response
// envelope shape (matches the convention used by profiles.go).
type PostEnvelope struct {
	Post Job `json:"post"`
}

// listEnvelope mirrors the `{posts: [...], pagination: {...}}` shape.
type listEnvelope struct {
	Posts      []Job `json:"posts"`
	Pagination struct {
		Page  int `json:"page"`
		Limit int `json:"limit"`
		Total int `json:"total"`
		Pages int `json:"pages"`
	} `json:"pagination"`
}

// Job is the canonical Zernio post object as returned by create / get
// / list. Field names mirror Zernio's exactly; we keep the verbose
// names rather than renaming so debugging against their docs stays
// straightforward.
type Job struct {
	ID           string            `json:"_id"`
	Status       JobStatus         `json:"status"`
	Content      string            `json:"content"`
	ScheduledFor *time.Time        `json:"scheduledFor,omitempty"`
	PublishedAt  *time.Time        `json:"publishedAt,omitempty"`
	Timezone     string            `json:"timezone,omitempty"`
	Platforms    []PlatformOutcome `json:"platforms"`
}

// ErrAlreadyPublished is returned by Cancel when Zernio reports the
// underlying job has already published — cancel is a no-op in that
// case and the next poll will land Published on Ogen's side.
var ErrAlreadyPublished = errors.New("zernio: job already published, cancel no-op")

// ErrDuplicateContent is returned by Submit when Zernio rejects the
// request as a same-content repeat within its 24h dedupe window. The
// caller is expected to recover by calling FindByContent to locate
// the existing job ID.
var ErrDuplicateContent = errors.New("zernio: duplicate content within 24h dedupe window")

// IsTerminalAPIError reports whether err is a Zernio HTTP error that
// the queue layer should NOT retry — 4xx (validation, auth, business
// rejection) other than 429 (rate limit, transient).
func IsTerminalAPIError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.Status == http.StatusTooManyRequests {
		return false
	}
	return apiErr.Status >= 400 && apiErr.Status < 500
}

// IsTransientAPIError reports whether err is something the queue
// layer should retry: 5xx, 429, or anything that isn't an APIError
// (typically network/timeout).
func IsTransientAPIError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err != nil
	}
	return apiErr.Status >= 500 || apiErr.Status == http.StatusTooManyRequests
}

// Submit creates a Zernio post. Returns the freshly created Job with
// its ID populated. On 409 (24h dedupe) returns ErrDuplicateContent
// so the queue handler can recover the existing job ID via
// FindByContent.
func (c *Client) Submit(ctx context.Context, req SubmitRequest) (*Job, error) {
	if c == nil {
		return nil, errors.New("zernio: client is disabled")
	}
	var env PostEnvelope
	err := c.do(ctx, http.MethodPost, "/posts", nil, req, &env)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateContent, apiErr.Message)
		}
		return nil, err
	}
	return &env.Post, nil
}

// Status fetches a single Zernio post by id.
func (c *Client) Status(ctx context.Context, jobID string) (*Job, error) {
	if c == nil {
		return nil, errors.New("zernio: client is disabled")
	}
	if jobID == "" {
		return nil, errors.New("zernio: jobID is required")
	}
	var env PostEnvelope
	if err := c.do(ctx, http.MethodGet, "/posts/"+url.PathEscape(jobID), nil, nil, &env); err != nil {
		return nil, err
	}
	return &env.Post, nil
}

// Cancel removes a scheduled Zernio post via DELETE — Zernio's
// documented cancellation path. Returns ErrAlreadyPublished when the
// job has already crossed the publish boundary (404 or a 409 with the
// "already published" message); the caller treats that as a no-op and
// lets the next poll resolve Published.
func (c *Client) Cancel(ctx context.Context, jobID string) error {
	if c == nil {
		return errors.New("zernio: client is disabled")
	}
	if jobID == "" {
		return errors.New("zernio: jobID is required")
	}
	err := c.do(ctx, http.MethodDelete, "/posts/"+url.PathEscape(jobID), nil, nil, nil)
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusNotFound:
			// Job is gone — either already published or never existed.
			// Either way, cancel is a no-op from Ogen's perspective.
			return ErrAlreadyPublished
		case http.StatusConflict:
			return ErrAlreadyPublished
		}
	}
	return err
}

// Retry asks Zernio to reattempt a failed publish. Used by the manual
// retry path (CON-69 §10) when an Ogen Post is being re-promoted out
// of Failed state and a Zernio job already exists for it.
func (c *Client) Retry(ctx context.Context, jobID string) (*Job, error) {
	if c == nil {
		return nil, errors.New("zernio: client is disabled")
	}
	if jobID == "" {
		return nil, errors.New("zernio: jobID is required")
	}
	var env PostEnvelope
	if err := c.do(ctx, http.MethodPost, "/posts/"+url.PathEscape(jobID)+"/retry", nil, nil, &env); err != nil {
		return nil, err
	}
	return &env.Post, nil
}

// FindByContent recovers the existing Zernio job after a 24h-dedupe
// 409 by listing scheduled posts within a recent window and matching
// on exact content. Used by the submit task's recovery path so the
// owning Ogen Post still ends up with a job_id to poll.
//
// Returns nil if no match is found within the lookback window — the
// caller should treat that as "submit truly failed" and surface to
// the Post Log.
func (c *Client) FindByContent(ctx context.Context, content string, lookback time.Duration) (*Job, error) {
	if c == nil {
		return nil, errors.New("zernio: client is disabled")
	}
	if lookback <= 0 {
		lookback = 24 * time.Hour
	}
	q := url.Values{}
	q.Set("status", string(JobStatusScheduled))
	q.Set("dateFrom", time.Now().UTC().Add(-lookback).Format(time.RFC3339))
	q.Set("limit", "100")
	q.Set("sortBy", "created-desc")

	var env listEnvelope
	if err := c.do(ctx, http.MethodGet, "/posts", q, nil, &env); err != nil {
		return nil, err
	}
	for i := range env.Posts {
		if env.Posts[i].Content == content {
			return &env.Posts[i], nil
		}
	}
	return nil, nil
}
