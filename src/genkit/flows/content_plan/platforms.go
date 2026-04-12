package content_plan

import (
	"context"
	"strings"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

func resolvePlatforms(ctx context.Context, targetPlatforms models.CampaignPlatforms, platformRepo repository.PlatformRepository) ([]resolvedPlatform, error) {
	all, err := platformRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]models.Platform, len(all))
	for _, p := range all {
		byID[p.ID] = p
	}

	platforms := make([]resolvedPlatform, 0, len(targetPlatforms))
	for _, tp := range targetPlatforms {
		p, ok := byID[tp.ID]
		if !ok {
			continue
		}

		// Resolve post type slugs for this platform entry.
		// If the campaign specifies post_types, use only those slugs and enforce
		// them in validateOutput. Otherwise fall back to all platform slugs with
		// no enforcement.
		var slugs []string
		var allowedSlugs []string
		if len(tp.PostTypes) > 0 {
			// Use only the slugs selected for this campaign. Unknown slugs are
			// passed through as-is so nothing is silently lost.
			slugs = append(slugs, tp.PostTypes...)
			allowedSlugs = slugs
		} else {
			for slug := range p.PostTypes {
				slugs = append(slugs, slug)
			}
			// allowedSlugs stays nil → no contentType enforcement
		}

		platforms = append(platforms, resolvedPlatform{
			ID:           p.ID,
			Name:         p.Name,
			PostTypes:    strings.Join(slugs, ", "),
			AllowedSlugs: allowedSlugs,
			Cadence:      p.Cadence,
			Constraints:  p.Constraints,
		})
	}
	return platforms, nil
}

