package repository

import (
	"context"

	"github.com/content-control-center/app/src/models"
)

// hydrateTags is a generic helper that attaches Tag objects to a slice of any
// entity type. Callers supply two accessor functions so the generic code never
// needs to know the concrete field names:
//
//   - getIDs  – returns the TagIDs slice for a given item (read-only copy)
//   - setTags – writes the resolved []models.Tag back onto a pointer to the item
func hydrateTags[T any](
	ctx context.Context,
	items []T,
	tagRepo TagRepository,
	getIDs func(T) []string,
	setTags func(*T, []models.Tag),
) error {
	for i := range items {
		setTags(&items[i], []models.Tag{})
	}

	seen := make(map[string]struct{})
	var allIDs []string
	for _, item := range items {
		for _, id := range getIDs(item) {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				allIDs = append(allIDs, id)
			}
		}
	}

	tagByID, err := tagRepo.GetByIDs(ctx, allIDs)
	if err != nil {
		return err
	}

	for i, item := range items {
		tags := make([]models.Tag, 0, len(getIDs(item)))
		for _, id := range getIDs(item) {
			if t, ok := tagByID[id]; ok {
				tags = append(tags, t)
			}
		}
		setTags(&items[i], tags)
	}
	return nil
}
