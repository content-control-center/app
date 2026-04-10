package content_plan

import (
	"context"
	"strings"

	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

func resolvePlatforms(ctx context.Context, platformIDs models.StringSlice, platformRepo repository.PlatformRepository) ([]resolvedPlatform, error) {
	all, err := platformRepo.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]models.Platform, len(all))
	for _, p := range all {
		byID[p.ID] = p
	}

	platforms := make([]resolvedPlatform, 0, len(platformIDs))
	for _, id := range platformIDs {
		p, ok := byID[id]
		if !ok {
			continue
		}
		typeNames := make([]string, 0, len(p.PostTypes))
		for _, name := range p.PostTypes {
			typeNames = append(typeNames, name)
		}
		platforms = append(platforms, resolvedPlatform{
			ID:          p.ID,
			Name:        p.Name,
			PostTypes:   strings.Join(typeNames, ", "),
			Cadence:     platformCadence(p.ID),
			Constraints: platformConstraints(p.ID),
		})
	}
	return platforms, nil
}

// platformCadence returns a human-readable cadence hint for a platform.
// todo: move to a database
func platformCadence(platformID string) string {
	switch platformID {
	case "linkedin":
		return "1–2 posts per week"
	case "x-twitter":
		return "3–5 posts per week"
	case "facebook":
		return "3–5 posts per week"
	case "instagram":
		return "4–7 posts per week"
	case "threads":
		return "3–5 posts per week"
	case "youtube":
		return "1 video per week"
	default:
		return "2–3 posts per week"
	}
}

// platformConstraints returns brief content guidance for a platform.
// todo: move to a database
func platformConstraints(platformID string) string {
	switch platformID {
	case "linkedin":
		return "professional tone; posts up to 3000 chars; articles up to 100k chars"
	case "x-twitter":
		return "posts max 280 chars; use threads for longer content"
	case "facebook":
		return "varied formats; posts up to 63k chars; balanced tone"
	case "instagram":
		return "visual-first; captions up to 2200 chars; strong opening line"
	case "threads":
		return "posts up to 500 chars; conversational tone"
	case "youtube":
		return "video-first; titles up to 100 chars; descriptions up to 5000 chars"
	default:
		return "adapt to platform norms"
	}
}
