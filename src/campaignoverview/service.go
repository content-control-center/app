package campaignoverview

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// statusOrder is the fixed display order for the byStatus breakdown, so the
// output is deterministic and reads lifecycle-first.
var statusOrder = []models.PostStatus{
	models.PostStatusDraft,
	models.PostStatusReadyForPublish,
	models.PostStatusScheduled,
	models.PostStatusScheduledForManualPublish,
	models.PostStatusPublished,
	models.PostStatusNotPublished,
	models.PostStatusFailed,
}

// Service computes campaign overviews from tenant-scoped repositories.
type Service struct {
	campaigns repository.CampaignRepository
	posts     repository.PostRepository
	platforms repository.PlatformRepository
}

// New builds a Service. The repositories carry tenant scoping, so Overview is
// safe to call in any tenant context.
func New(campaigns repository.CampaignRepository, posts repository.PostRepository, platforms repository.PlatformRepository) *Service {
	return &Service{campaigns: campaigns, posts: posts, platforms: platforms}
}

// Overview loads the campaign (with its type + phases hydrated) and its posts,
// then computes the brief recap, per-phase post counts, and the status /
// platform / content-type distribution. Returns ErrNotFound when the campaign
// does not exist in the caller's tenant.
func (s *Service) Overview(ctx context.Context, campaignID string) (*Overview, error) {
	campaign, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load campaign: %w", err)
	}

	posts, err := s.posts.ListByCampaign(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}

	// Platform-name resolution is best-effort: on failure the byPlatform buckets
	// fall back to platform ids rather than failing the whole overview.
	names := map[string]string{}
	if platforms, perr := s.platforms.List(ctx); perr == nil {
		names = make(map[string]string, len(platforms))
		for _, p := range platforms {
			names[p.ID] = p.Name
		}
	}

	ov := buildOverview(campaign, posts, names)
	ov.GeneratedAt = time.Now().UTC()
	return ov, nil
}

// buildOverview is the pure aggregation (no I/O), unit-testable with hand-built
// inputs. GeneratedAt is stamped by the caller.
func buildOverview(campaign *models.Campaign, posts []models.Post, platformNames map[string]string) *Overview {
	typeName := campaign.CampaignTypeID
	var phaseDefs []models.CampaignTypePhase
	if campaign.CampaignType != nil {
		if campaign.CampaignType.Name != "" {
			typeName = campaign.CampaignType.Name
		}
		phaseDefs = campaign.CampaignType.Phases
	}

	knownPhase := make(map[string]bool, len(phaseDefs))
	for _, ph := range phaseDefs {
		knownPhase[ph.ID] = true
	}

	var (
		phaseCount    = map[string]int{}
		statusCount   = map[models.PostStatus]int{}
		platformCount = map[string]int{}
		typeCount     = map[string]int{}
		unassigned    int
	)
	for _, p := range posts {
		if p.CampaignTypePhaseID != nil && knownPhase[*p.CampaignTypePhaseID] {
			phaseCount[*p.CampaignTypePhaseID]++
		} else {
			unassigned++
		}
		statusCount[p.Status]++
		platformCount[p.PlatformID]++
		typeCount[p.PlatformPostType]++
	}

	phases := make([]PhaseInfo, 0, len(phaseDefs))
	for _, ph := range phaseDefs {
		phases = append(phases, PhaseInfo{
			ID:        ph.ID,
			Sequence:  ph.Sequence,
			Name:      ph.Name,
			Purpose:   ph.Purpose,
			PostCount: phaseCount[ph.ID],
		})
	}

	return &Overview{
		CampaignID: campaign.ID,
		Name:       campaign.Name,
		Status:     string(campaign.Status),
		Type:       typeName,
		Language:   campaign.Language,
		Brief: Brief{
			Description:    campaign.Description,
			TargetPersona:  campaign.TargetPersona,
			KeyMessages:    campaign.KeyMessages,
			ToneGuidelines: campaign.ToneGuidelines,
		},
		Phases:     phases,
		TotalPosts: len(posts),
		Distribution: Distribution{
			UnassignedPhasePostCount: unassigned,
			ByStatus:                 statusBuckets(statusCount),
			ByPlatform:               platformBuckets(platformCount, platformNames),
			ByContentType:            slugBuckets(typeCount),
		},
	}
}

// statusBuckets emits non-zero statuses in the fixed lifecycle order.
func statusBuckets(counts map[models.PostStatus]int) []Bucket {
	out := make([]Bucket, 0, len(counts))
	for _, st := range statusOrder {
		if n := counts[st]; n > 0 {
			out = append(out, Bucket{Key: string(st), Label: string(st), Count: n})
		}
	}
	return out
}

// platformBuckets resolves ids to names (empty id → "None", unknown id → the id)
// and orders by count desc, then label asc.
func platformBuckets(counts map[string]int, names map[string]string) []Bucket {
	out := make([]Bucket, 0, len(counts))
	for id, n := range counts {
		label := names[id]
		switch {
		case id == "":
			label = "None"
		case label == "":
			label = id
		}
		out = append(out, Bucket{Key: id, Label: label, Count: n})
	}
	sortBuckets(out)
	return out
}

// slugBuckets orders content-type slugs by count desc, then slug asc (empty
// slug → "None").
func slugBuckets(counts map[string]int) []Bucket {
	out := make([]Bucket, 0, len(counts))
	for slug, n := range counts {
		label := slug
		if slug == "" {
			label = "None"
		}
		out = append(out, Bucket{Key: slug, Label: label, Count: n})
	}
	sortBuckets(out)
	return out
}

// sortBuckets applies the deterministic count-desc, label-asc order.
func sortBuckets(b []Bucket) {
	sort.SliceStable(b, func(i, j int) bool {
		if b[i].Count != b[j].Count {
			return b[i].Count > b[j].Count
		}
		return b[i].Label < b[j].Label
	})
}
