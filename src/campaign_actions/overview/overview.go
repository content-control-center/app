// Package overview computes a read-only "quick overview" of a campaign
// (CON-113): its brief, phases with per-phase post counts, and content
// distribution by status, platform, and content type. The same service backs
// both the Campaign Assistant's getCampaignOverview tool and the
// GET /api/campaigns/:id/overview REST endpoint.
package overview

import (
	"errors"
	"time"
)

// ErrNotFound is returned by Service.Overview when the campaign does not exist
// (or belongs to another tenant). Handlers map it to 404.
var ErrNotFound = errors.New("campaign not found")

// Overview is the read-only snapshot returned to the assistant tool and the
// REST endpoint.
type Overview struct {
	CampaignID   string       `json:"campaignId"`
	Name         string       `json:"name"`
	Status       string       `json:"status"`
	Type         string       `json:"type"` // campaign type name
	Language     string       `json:"language"`
	Brief        Brief        `json:"brief"`
	Phases       []PhaseInfo  `json:"phases"` // ordered by sequence
	TotalPosts   int          `json:"totalPosts"`
	Distribution Distribution `json:"distribution"`
	GeneratedAt  time.Time    `json:"generatedAt"`
}

// Brief recaps the campaign's brief fields.
type Brief struct {
	Description    string `json:"description"`
	TargetPersona  string `json:"targetPersona"`
	KeyMessages    string `json:"keyMessages"`
	ToneGuidelines string `json:"toneGuidelines"`
}

// PhaseInfo is one campaign phase plus how many posts are assigned to it — the
// content distribution across phases.
type PhaseInfo struct {
	ID        string `json:"id"`
	Sequence  int    `json:"sequence"`
	Name      string `json:"name"`
	Purpose   string `json:"purpose"`
	PostCount int    `json:"postCount"`
}

// Distribution holds the flat post breakdowns. Counts reconcile: TotalPosts
// equals the sum of each breakdown, and the sum of phase PostCounts plus
// UnassignedPhasePostCount.
type Distribution struct {
	// UnassignedPhasePostCount counts posts with no phase (or a stale phase id).
	UnassignedPhasePostCount int      `json:"unassignedPhasePostCount"`
	ByStatus                 []Bucket `json:"byStatus"`
	ByPlatform               []Bucket `json:"byPlatform"`
	ByContentType            []Bucket `json:"byContentType"`
}

// Bucket is one entry in a distribution breakdown.
type Bucket struct {
	Key   string `json:"key"`   // status string / platform id / post-type slug
	Label string `json:"label"` // human label (platform name; else the key or "None")
	Count int    `json:"count"`
}
