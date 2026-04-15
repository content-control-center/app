//go:build integration

package integration_test

import (
	"context"
	"database/sql"

	"github.com/alephbet-ai/llama-genkit-embedder/llama"
	"github.com/firebase/genkit/go/genkit"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/genkit/flows"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

// embedServerURL points at the llama-embedserver exposed by docker-compose.integration.yml.
const embedServerURL = "http://localhost:9003"

var _ = Describe("Asset embedding flow", Ordered, func() {
	var (
		ctx     context.Context
		db      *bun.DB
		repo    repository.AssetsEmbeddingsRepository
		assetID string
	)

	BeforeAll(func() {
		ctx = context.Background()
		db = mustOpenIntegrationDB()
		repo = repository.NewAssetsEmbeddingRepository(db)

		// Initialise Genkit and the llama embedder once for the whole suite.
		plugin := llama.New(llama.Config{LlamaEmbedServerAddress: embedServerURL})
		g := genkit.Init(ctx, genkit.WithPlugins(plugin))
		embedder, err := plugin.DefineEmbedder(g)
		Expect(err).NotTo(HaveOccurred(), "llama-embedserver must be running at %s", embedServerURL)
		flows.Init(g, embedder, repo)

		// Seed a user and a asset to satisfy foreign-key constraints.
		userID, err := models.NewID()
		Expect(err).NotTo(HaveOccurred())
		user := &models.User{
			ID:           userID,
			Name:         "Integration Tester",
			Email:        "integration@test.local",
			PasswordHash: "placeholder",
		}
		_, err = db.NewInsert().Model(user).Exec(ctx)
		Expect(err).NotTo(HaveOccurred())

		assetID, err = models.NewID()
		Expect(err).NotTo(HaveOccurred())
		asset := &models.Asset{
			ID:        assetID,
			Title:     "Integration Asset",
			Content:   blocknoteDoc("Hello, embedding world!"),
			CreatedBy: userID,
		}
		_, err = db.NewInsert().Model(asset).Exec(ctx)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		_, _ = db.NewDelete().TableExpr("assets_embeddings").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("assets").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(ctx)
	})

	// ── Create ───────────────────────────────────────────────────────────────

	Describe("on asset create", func() {
		It("stores a non-empty embedding vector", func() {
			_, err := flows.EmbedAssetFlow.Run(ctx, flows.EmbedAssetInput{
				AssetID: assetID,
				Title:   "Integration Asset",
				Content: blocknoteDoc("Hello, embedding world!"),
			})
			Expect(err).NotTo(HaveOccurred())

			vector, model, err := repo.GetByAssetID(ctx, assetID)
			Expect(err).NotTo(HaveOccurred())
			Expect(vector).To(HaveLen(768))
			Expect(model).NotTo(BeEmpty())
		})
	})

	// ── Update ───────────────────────────────────────────────────────────────

	Describe("on asset update", func() {
		It("replaces the embedding with a new vector", func() {
			// Run the flow with updated content.
			_, err := flows.EmbedAssetFlow.Run(ctx, flows.EmbedAssetInput{
				AssetID: assetID,
				Title:   "Integration Asset — Revised",
				Content: blocknoteDoc("The content has been updated."),
			})
			Expect(err).NotTo(HaveOccurred())

			// Embedding row must still exist and be valid.
			vector, model, err := repo.GetByAssetID(ctx, assetID)
			Expect(err).NotTo(HaveOccurred())
			Expect(vector).NotTo(BeEmpty())
			Expect(model).NotTo(BeEmpty())
		})

		It("returns sql.ErrNoRows for an unknown asset ID", func() {
			_, _, err := repo.GetByAssetID(ctx, "nonexistent-id")
			Expect(err).To(MatchError(sql.ErrNoRows))
		})
	})
})

// blocknoteDoc returns a minimal single-paragraph BlockNote JSON document.
func blocknoteDoc(text string) string {
	return `[{"type":"paragraph","content":[{"type":"text","text":"` + text + `","styles":{}}],"children":[]}]`
}
