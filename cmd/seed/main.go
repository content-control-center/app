package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/database"
	"github.com/content-control-center/app/src/embedding"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

// seedDir is resolved relative to the working directory, which is the repo
// root when invoked via `make seed`.
const seedDir = "seeds"

// ---------- raw fixture types ----------

type seedTag struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type seedAsset struct {
	Title   string          `json:"title"`
	Content json.RawMessage `json:"content"` // BlockNote JSON array — stored as string in DB
	Tags    []string        `json:"tags"`    // tag names, resolved to IDs at runtime
}

type seedCampaign struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	TargetPersona      string   `json:"target_persona"`
	KeyMessages        string   `json:"key_messages"`
	ToneGuidelines     string   `json:"tone_guidelines"`
	CampaignTypeID     string   `json:"campaign_type_id"`
	Status             string   `json:"status"`
	UseAssets          bool     `json:"use_assets"`
	AssetIndices       []int    `json:"asset_indices"`       // 0-based indices into assets slice
	TargetPlatformIDs  []string `json:"target_platform_ids"` // literal platform IDs (seeded by migrations)
	EstimatedPostCount *int     `json:"estimated_post_count"`
	Budget             *float64 `json:"budget"`
	Currency           string   `json:"currency"`
	Language           string   `json:"language"`
	Tags               []string `json:"tags"` // tag names, resolved to IDs at runtime
	StartDate          *string  `json:"start_date"`
	EndDate            *string  `json:"end_date"`
}

type seedPost struct {
	CampaignIndex       int     `json:"campaign_index"` // 0-based index into campaigns slice
	PlatformID          string  `json:"platform_id"`
	PlatformPostType    string  `json:"platform_post_type"`
	Title               string  `json:"title"`
	Content             string  `json:"content"`
	Status              string  `json:"status"`
	CTAType             string  `json:"cta_type"`
	CTAUrl              string  `json:"cta_url"`
	TargetAudienceNotes string  `json:"target_audience_notes"`
	AssetIndices        []int   `json:"asset_indices"` // 0-based indices into assets slice
	ScheduledAt         *string `json:"scheduled_at"`
}

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := ensureDBDir(cfg.DSN); err != nil {
		log.Fatalf("prepare database directory: %v", err)
	}

	db, err := database.New(cfg.DSN, cfg.Debug)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(ctx, db); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// Guard: abort if data already exists to avoid duplicate seeds.
	userRepo := repository.NewUserRepository(db)
	existing, err := userRepo.List(ctx)
	if err != nil {
		log.Fatalf("check users: %v", err)
	}
	if len(existing) > 0 {
		log.Fatal("database already contains users — aborting to avoid duplicate seed data")
	}

	// ---------- load fixtures ----------
	tags := mustLoad[[]seedTag]("tags.json")
	assets := mustLoad[[]seedAsset]("assets.json")
	campaigns := mustLoad[[]seedCampaign]("campaigns.json")
	posts := mustLoad[[]seedPost]("posts.json")

	// ---------- seed user ----------
	userID, err := models.NewID()
	if err != nil {
		log.Fatalf("generate user id: %v", err)
	}
	hash, err := models.HashPassword("Seed1234!")
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}
	seedUser := &models.User{
		ID:           userID,
		Name:         "Seed User",
		Email:        "seed@example.com",
		PasswordHash: hash,
	}
	if err := userRepo.Create(ctx, seedUser); err != nil {
		log.Fatalf("create seed user: %v", err)
	}
	log.Printf("user:     seed@example.com  (password: Seed1234!)  id=%s", userID)

	// ---------- seed tags ----------
	tagRepo := repository.NewTagRepository(db)
	tagIDs := make(map[string]string, len(tags)) // name → id
	for _, st := range tags {
		id, err := models.NewID()
		if err != nil {
			log.Fatalf("generate tag id: %v", err)
		}
		t := &models.Tag{ID: id, Name: st.Name, Color: st.Color, CreatedBy: userID}
		if err := tagRepo.Create(ctx, t); err != nil {
			log.Fatalf("create tag %q: %v", st.Name, err)
		}
		tagIDs[st.Name] = id
	}
	log.Printf("tags:     %d created", len(tags))

	// ---------- seed assets ----------
	assetRepo := repository.NewAssetRepository(db, tagRepo, repository.NewAssetFileRepository(db))
	assetIDs := make([]string, 0, len(assets))
	for _, sp := range assets {
		id, err := models.NewID()
		if err != nil {
			log.Fatalf("generate asset id: %v", err)
		}
		p := &models.Asset{
			ID:        id,
			Title:     sp.Title,
			Content:   string(sp.Content),
			TagIDs:    resolveTagIDs(sp.Tags, tagIDs),
			CreatedBy: userID,
		}
		if err := assetRepo.Create(ctx, p); err != nil {
			log.Fatalf("create asset %q: %v", sp.Title, err)
		}
		assetIDs = append(assetIDs, id)
	}
	log.Printf("assets:   %d created", len(assets))

	// ---------- seed campaigns ----------
	campaignRepo := repository.NewCampaignRepository(db, tagRepo)
	campaignIDs := make([]string, 0, len(campaigns))
	for _, sc := range campaigns {
		id, err := models.NewID()
		if err != nil {
			log.Fatalf("generate campaign id: %v", err)
		}
		c := &models.Campaign{
			ID:                 id,
			Name:               sc.Name,
			Description:        sc.Description,
			TargetPersona:      sc.TargetPersona,
			KeyMessages:        sc.KeyMessages,
			ToneGuidelines:     sc.ToneGuidelines,
			CampaignTypeID:     sc.CampaignTypeID,
			Status:             models.CampaignStatus(sc.Status),
			UseAssets:          sc.UseAssets,
			AssetIDs:          resolveIndices(sc.AssetIndices, assetIDs, "asset"),
			TargetPlatformIDs:  models.StringSlice(sc.TargetPlatformIDs),
			EstimatedPostCount: sc.EstimatedPostCount,
			Budget:             sc.Budget,
			Currency:           sc.Currency,
			Language:           sc.Language,
			TagIDs:             resolveTagIDs(sc.Tags, tagIDs),
			StartDate:          parseTime(sc.StartDate, sc.Name, "start_date"),
			EndDate:            parseTime(sc.EndDate, sc.Name, "end_date"),
			CreatedBy:          userID,
		}
		if err := campaignRepo.Create(ctx, c); err != nil {
			log.Fatalf("create campaign %q: %v", sc.Name, err)
		}
		campaignIDs = append(campaignIDs, id)
	}
	log.Printf("campaigns: %d created", len(campaigns))

	// ---------- seed posts ----------
	postRepo := repository.NewPostRepository(db)
	for _, sp := range posts {
		if sp.CampaignIndex < 0 || sp.CampaignIndex >= len(campaignIDs) {
			log.Fatalf("post %q: campaign_index %d out of range", sp.Title, sp.CampaignIndex)
		}
		id, err := models.NewID()
		if err != nil {
			log.Fatalf("generate post id: %v", err)
		}
		post := &models.Post{
			ID:                  id,
			CampaignID:          campaignIDs[sp.CampaignIndex],
			PlatformID:          sp.PlatformID,
			PlatformPostType:    sp.PlatformPostType,
			Title:               sp.Title,
			Content:             sp.Content,
			MediaURLs:           models.StringSlice{},
			Status:              models.PostStatus(sp.Status),
			CTAType:             models.PostCTAType(sp.CTAType),
			CTAUrl:              sp.CTAUrl,
			TargetAudienceNotes: sp.TargetAudienceNotes,
			UsedAssetIDs:       resolveIndices(sp.AssetIndices, assetIDs, "asset"),
			ScheduledAt:         parseTime(sp.ScheduledAt, sp.Title, "scheduled_at"),
			CreatedBy:           userID,
		}
		if err := postRepo.Create(ctx, post); err != nil {
			log.Fatalf("create post %q: %v", sp.Title, err)
		}
	}
	log.Printf("posts:    %d created", len(posts))

	// ---------- embeddings ----------
	// Use a short retry window so the seed command doesn't stall when the embed
	// server is not running. Embeddings are optional — the seed succeeds either way.
	embeddingRepo := repository.NewAssetsEmbeddingRepository(db)
	onSave, _, err := embedding.InitWithOptions(ctx, cfg.EmbedServerURL, 2, 3*time.Second, embeddingRepo)
	if err != nil {
		log.Printf("embeddings: server unavailable (%v) — skipping", err)
	}
	if onSave == nil {
		log.Println("embeddings: skipped (set EMBED_SERVER_URL to generate)")
	} else {
		log.Printf("embeddings: generating for %d assets...", len(assets))
		for i, sp := range assets {
			onSave(assetIDs[i], sp.Title, string(sp.Content))
			log.Printf("embeddings: [%d/%d] %s", i+1, len(assets), sp.Title)
		}
		log.Println("embeddings: done")
	}

	fmt.Println()
	fmt.Println("Seed complete.")
	fmt.Printf("  Login: seed@example.com / Seed1234!\n")
}

// ---------- helpers ----------

func mustLoad[T any](filename string) T {
	path := seedDir + "/" + filename
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		log.Fatalf("parse %s: %v", path, err)
	}
	return v
}

func resolveTagIDs(names []string, tagIDs map[string]string) models.StringSlice {
	ids := make(models.StringSlice, 0, len(names))
	for _, name := range names {
		id, ok := tagIDs[name]
		if !ok {
			log.Fatalf("unknown tag %q — check seeds/tags.json", name)
		}
		ids = append(ids, id)
	}
	return ids
}

func resolveIndices(indices []int, ids []string, kind string) models.StringSlice {
	result := make(models.StringSlice, 0, len(indices))
	for _, idx := range indices {
		if idx < 0 || idx >= len(ids) {
			log.Fatalf("%s index %d out of range (have %d)", kind, idx, len(ids))
		}
		result = append(result, ids[idx])
	}
	return result
}

// ensureDBDir creates the parent directory of the SQLite file so that the
// driver can create the database on first run. It is a no-op for in-memory
// DSNs and for DSNs whose directory already exists.
func ensureDBDir(dsn string) error {
	// Strip the optional "file:" URI prefix.
	path := strings.TrimPrefix(dsn, "file:")
	// Strip query parameters.
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	// Skip in-memory databases.
	if path == "" || path == ":memory:" {
		return nil
	}
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o755)
}

func parseTime(s *string, entity, field string) *time.Time {
	if s == nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		log.Fatalf("%s %q: parse %s: %v", entity, field, *s, err)
	}
	return &t
}
