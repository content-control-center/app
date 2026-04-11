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
			Cadence:     p.Cadence,
			Constraints: p.Constraints,
		})
	}
	return platforms, nil
}

