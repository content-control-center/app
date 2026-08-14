package repository_test

import (
	"testing"
	"time"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// TestAssetDelete_ScrubsDanglingReferences verifies that deleting an asset also
// removes its id from every campaign.asset_ids and post.used_asset_ids (CON-214),
// so a deleted asset can never linger as a dangling reference that later
// hard-fails content generation ("asset %q not found").
func TestAssetDelete_ScrubsDanglingReferences(t *testing.T) {
	db := openMigratedDB(t)
	ctx := tenantCtx()
	now := time.Now().UTC()

	const delID, keepID = "asset-del", "asset-keep"

	asset := &models.Asset{
		ID:        delID,
		Title:     "To delete",
		Content:   "body",
		Status:    models.AssetStatusReady,
		TagIDs:    models.StringSlice{},
		CreatedBy: "user-1",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := db.NewInsert().Model(asset).Exec(ctx); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	campaign := &models.Campaign{
		ID:              "camp-1",
		Name:            "C",
		UseAssets:       true,
		AssetIDs:        models.StringSlice{delID, keepID},
		TargetPlatforms: models.CampaignPlatforms{},
		PublishingDays:  models.StringSlice{},
		TagIDs:          models.StringSlice{},
		Status:          models.StatusActive,
		CreatedBy:       "user-1",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if _, err := db.NewInsert().Model(campaign).Exec(ctx); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	post := &models.Post{
		ID:           "post-1",
		CampaignID:   "camp-1",
		Title:        "P",
		Content:      "body",
		MediaURLs:    models.StringSlice{},
		UsedAssetIDs: models.StringSlice{delID, keepID},
		Status:       models.PostStatusDraft,
		CTAType:      models.CTATypeNone,
		CreatedBy:    "user-1",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if _, err := db.NewInsert().Model(post).Exec(ctx); err != nil {
		t.Fatalf("seed post: %v", err)
	}

	repo := repository.NewAssetRepository(db, nil, nil)
	deleted, err := repo.Delete(ctx, delID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !deleted {
		t.Fatal("expected deleted=true")
	}

	// The deleted id is scrubbed from both back-references; the surviving id stays.
	gotCampaign := new(models.Campaign)
	if err := db.NewSelect().Model(gotCampaign).Where("c.id = ?", "camp-1").Scan(ctx); err != nil {
		t.Fatalf("reload campaign: %v", err)
	}
	if got := []string(gotCampaign.AssetIDs); len(got) != 1 || got[0] != keepID {
		t.Errorf("campaign.AssetIDs = %v, want [%s]", got, keepID)
	}

	gotPost := new(models.Post)
	if err := db.NewSelect().Model(gotPost).Where("po.id = ?", "post-1").Scan(ctx); err != nil {
		t.Fatalf("reload post: %v", err)
	}
	if got := []string(gotPost.UsedAssetIDs); len(got) != 1 || got[0] != keepID {
		t.Errorf("post.UsedAssetIDs = %v, want [%s]", got, keepID)
	}
}
