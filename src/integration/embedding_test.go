//go:build integration

package integration_test

import (
	"context"

	"github.com/alephbet-ai/llama-genkit-embedder/llama"
	"github.com/firebase/genkit/go/genkit"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/genkit/flows"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// embedServerURL points at the llama-embedserver exposed by docker-compose.integration.yml.
const embedServerURL = "http://localhost:9003"

var _ = Describe("Asset embedding flow", Ordered, func() {
	var (
		ctx     context.Context
		db      *bun.DB
		repo    repository.AssetChunksRepository
		assetID string
	)

	BeforeAll(func() {
		ctx = context.Background()
		db = mustOpenIntegrationDB()
		repo = repository.NewAssetChunksRepository(db)

		// Initialise Genkit and the llama embedder once for the whole suite.
		plugin := llama.New(llama.Config{LlamaEmbedServerAddress: embedServerURL})
		g := genkit.Init(ctx, genkit.WithPlugins(plugin))
		embedder, err := plugin.DefineEmbedder(g)
		Expect(err).NotTo(HaveOccurred(), "llama-embedserver must be running at %s", embedServerURL)
		flows.Init(g, embedder, repo, nil)

		// Seed a user and an asset to satisfy foreign-key constraints.
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
		_, _ = db.NewDelete().TableExpr("assets_chunks").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("assets").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(ctx)
	})

	// ── Create ───────────────────────────────────────────────────────────────

	Describe("on asset create", func() {
		It("stores one or more chunks with non-empty embedding vectors", func() {
			_, err := flows.EmbedAssetFlow.Run(ctx, flows.EmbedAssetInput{
				AssetID: assetID,
				Title:   "Integration Asset",
				Content: blocknoteDoc("Hello, embedding world!"),
			})
			Expect(err).NotTo(HaveOccurred())

			chunks, err := repo.GetByAssetID(ctx, assetID)
			Expect(err).NotTo(HaveOccurred())
			Expect(chunks).NotTo(BeEmpty())
			Expect(chunks[0].Embedding).NotTo(BeEmpty())
			Expect(chunks[0].Model).NotTo(BeEmpty())
			Expect(chunks[0].ChunkIndex).To(Equal(0))

			// Verify the embedding decodes to the expected dimension (768 for embeddinggemma-300m).
			vec := flows.DecodeVector(chunks[0].Embedding)
			Expect(vec).To(HaveLen(768))
		})
	})

	// ── Update ───────────────────────────────────────────────────────────────

	Describe("on asset update", func() {
		It("replaces the chunks with fresh vectors", func() {
			_, err := flows.EmbedAssetFlow.Run(ctx, flows.EmbedAssetInput{
				AssetID: assetID,
				Title:   "Integration Asset — Revised",
				Content: blocknoteDoc("The content has been updated."),
			})
			Expect(err).NotTo(HaveOccurred())

			chunks, err := repo.GetByAssetID(ctx, assetID)
			Expect(err).NotTo(HaveOccurred())
			Expect(chunks).NotTo(BeEmpty())
			Expect(flows.DecodeVector(chunks[0].Embedding)).NotTo(BeEmpty())
			Expect(chunks[0].Model).NotTo(BeEmpty())
		})

		It("returns an empty slice for an unknown asset ID", func() {
			chunks, err := repo.GetByAssetID(ctx, "nonexistent-id")
			Expect(err).NotTo(HaveOccurred())
			Expect(chunks).To(BeEmpty())
		})
	})
})

// blocknoteDoc returns a minimal single-paragraph BlockNote JSON document.
func blocknoteDoc(text string) string {
	return `[{"type":"paragraph","content":[{"type":"text","text":"` + text + `","styles":{}}],"children":[]}]`
}
