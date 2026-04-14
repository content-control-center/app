package content_plan

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

func validateInput(ctx context.Context, campaignID string, campaignRepo repository.CampaignRepository, pieceRepo repository.PieceRepository) (*models.Campaign, error) {
	c, err := campaignRepo.GetByID(ctx, campaignID)
	if err != nil {
		return nil, err
	}

	var missing []string
	if c.Name == "" {
		missing = append(missing, "name")
	}
	if c.Description == "" {
		missing = append(missing, "description")
	}
	if c.TargetPersona == "" {
		missing = append(missing, "target_persona")
	}
	if c.KeyMessages == "" {
		missing = append(missing, "key_messages")
	}
	if c.ToneGuidelines == "" {
		missing = append(missing, "tone_guidelines")
	}
	if c.CampaignTypeID == "" {
		missing = append(missing, "campaign_type_id")
	}
	if c.StartDate == nil {
		missing = append(missing, "start_date")
	}
	if c.EndDate == nil {
		missing = append(missing, "end_date")
	}
	if len(c.TargetPlatforms) == 0 {
		missing = append(missing, "target_platforms")
	}
	if len(missing) > 0 {
		return nil, &ValidationError{Msg: "missing required campaign fields: " + strings.Join(missing, ", ")}
	}

	if !c.StartDate.Before(*c.EndDate) {
		return nil, &ValidationError{Msg: "start_date must be before end_date"}
	}
	if c.EndDate.Sub(*c.StartDate) < 24*time.Hour {
		return nil, &ValidationError{Msg: "date range must be at least 1 day"}
	}

	// Verify specific piece IDs exist when UsePieces is true and IDs are given.
	if c.UsePieces && len(c.PiecesIDs) > 0 {
		for _, id := range c.PiecesIDs {
			if _, err := pieceRepo.GetByID(ctx, id); err != nil {
				return nil, &ValidationError{Msg: fmt.Sprintf("piece %q not found", id)}
			}
		}
	}

	return c, nil
}

func validateOutput(posts []DraftPost, campaign *models.Campaign, platforms []resolvedPlatform) ([]DraftPost, []string) {
	validPlatformIDs := make(map[string]bool, len(platforms))
	// platformAllowedSlugs is non-nil only when the campaign explicitly
	// constrains post types for that platform.
	platformAllowedSlugs := make(map[string]map[string]bool)
	for _, p := range platforms {
		validPlatformIDs[p.ID] = true
		if len(p.AllowedSlugs) > 0 {
			m := make(map[string]bool, len(p.AllowedSlugs))
			for _, s := range p.AllowedSlugs {
				m[s] = true
			}
			platformAllowedSlugs[p.ID] = m
		}
	}

	startDate := campaign.StartDate.Format("2006-01-02")
	endDate := campaign.EndDate.Format("2006-01-02")

	valid := make([]DraftPost, 0, len(posts))
	var warnings []string

	for _, post := range posts {
		if !validPlatformIDs[post.PlatformID] {
			warnings = append(warnings, fmt.Sprintf("post %q dropped: unknown platformId %q", post.Title, post.PlatformID))
			continue
		}
		if allowed, ok := platformAllowedSlugs[post.PlatformID]; ok {
			if !allowed[post.ContentType] {
				warnings = append(warnings, fmt.Sprintf("post %q dropped: contentType %q not allowed for platform %q", post.Title, post.ContentType, post.PlatformID))
				continue
			}
		}
		d := post.PublishDate
		if d < startDate || d > endDate {
			warnings = append(warnings, fmt.Sprintf("post %q dropped: publishDate %q is outside campaign range", post.Title, d))
			continue
		}
		valid = append(valid, post)
	}

	return valid, warnings
}
