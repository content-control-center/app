package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/handlers"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// CON-245: campaigns and posts can bind a specific brand voice + audience, and a
// post's own voice/audience is settable via the targeted PUT /:id/brand.
var _ = Describe("Brand bindings (CON-245)", Ordered, func() {
	var (
		app        *fiber.App
		db         *bun.DB
		authCookie *http.Cookie
		brandRepo  repository.BrandRepository
		campaignID string
		postID     string
	)

	BeforeAll(func() { db = mustOpenTestDBWithMigrations() })

	BeforeEach(func() {
		app = fiber.New(fiber.Config{
			ErrorHandler: func(c *fiber.Ctx, err error) error {
				code := fiber.StatusInternalServerError
				if e, ok := err.(*fiber.Error); ok {
					code = e.Code
				}
				return c.Status(code).JSON(fiber.Map{"error": err.Error()})
			},
		})
		userRepo := repository.NewUserRepository(db)
		sessionRepo := repository.NewSessionRepository(db)
		auth := handlers.RequireAuth(sessionRepo, userRepo, testCookieName)
		brandRepo = repository.NewBrandRepository(db)
		campaignTypeRepo := repository.NewCampaignTypeRepository(db)
		campaignRepo := repository.NewCampaignRepository(db, repository.NewTagRepository(db), repository.NewPlatformRepository(db), campaignTypeRepo)
		postRepo := repository.NewPostRepository(db)

		handlers.NewSessionsHandler(userRepo, repository.NewAccountRepository(db), sessionRepo, testCookieName, false).Register(app)
		ch := handlers.NewCampaignsHandler(campaignRepo, campaignTypeRepo, auth, nil, nil, nil, nil, nil)
		ch.SetBrandRepo(brandRepo)
		ch.Register(app)
		ph := handlers.NewPostsHandler(postRepo, repository.NewPostVersionRepository(db), repository.NewPostAssistantMessageRepository(db), repository.NewPlatformRepository(db), repository.NewPostAttachmentRepository(db), auth, nil, nil)
		ph.SetBrandRepo(brandRepo)
		ph.Register(app)

		user := seedTenantUser(db, "Admin", "admin@example.com", "admin-password")

		loginBody, _ := json.Marshal(fiber.Map{"email": "admin@example.com", "password": "admin-password"})
		loginReq := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(loginBody))
		loginReq.Header.Set("Content-Type", "application/json")
		loginResp, err := app.Test(loginReq)
		Expect(err).NotTo(HaveOccurred())
		Expect(loginResp.StatusCode).To(Equal(fiber.StatusCreated))
		authCookie = loginResp.Cookies()[0]

		// Seed brand voice + audience, a campaign, and a post directly.
		ctx := tenantCtx()
		Expect(brandRepo.CreateVoice(ctx, &models.BrandVoice{
			ID: "v-1", Name: "Dry British",
			Samples: models.StringSlice{}, ChannelNotes: models.StringMap{},
			Origin: models.BrandOrigin{Kind: "blank"},
		})).To(Succeed())
		Expect(brandRepo.CreateAudience(ctx, &models.BrandAudience{
			ID: "a-1", Name: "Sceptics", Origin: models.BrandOrigin{Kind: "blank"},
		})).To(Succeed())

		campaignID = "camp-1"
		Expect(campaignRepo.Create(ctx, &models.Campaign{
			ID: campaignID, Name: "C", CampaignTypeID: seededCampaignTypeID,
			AssetIDs: models.StringSlice{}, TargetPlatforms: models.CampaignPlatforms{},
			TagIDs: models.StringSlice{}, PublishingDays: models.StringSlice{},
			Status: models.StatusDraft, PublishingTime: "09:00", SpreadMinutes: 15,
			GoalCadence: "month", CreatedBy: user.ID,
		})).To(Succeed())

		postID = "post-1"
		Expect(postRepo.Create(ctx, &models.Post{
			ID: postID, CampaignID: campaignID, Content: "hi",
			Status: models.PostStatusDraft, CTAType: models.CTATypeNone,
			MediaURLs: models.StringSlice{}, UsedAssetIDs: models.StringSlice{},
			CreatedBy: user.ID,
		})).To(Succeed())
	})

	AfterEach(func() {
		for _, tbl := range []string{"posts", "campaigns", "brand_voices", "brand_audiences", "sessions", "users", "accounts"} {
			_, err := db.NewDelete().TableExpr(tbl).Where("1 = 1").Exec(context.Background())
			Expect(err).NotTo(HaveOccurred())
		}
	})

	do := func(method, path string, body any) *http.Response {
		GinkgoHelper()
		var r *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		} else {
			r = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, path, r)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	strptr := func(s string) *string { return &s }

	Describe("PUT /api/posts/:id/brand", func() {
		It("sets a post's own voice + audience and round-trips", func() {
			resp := do("PUT", "/api/posts/"+postID+"/brand", fiber.Map{
				"brand_voice_id": "v-1", "brand_audience_id": "a-1",
			})
			Expect(resp.StatusCode).To(Equal(200))
			var p models.Post
			Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
			Expect(p.BrandVoiceID).To(Equal(strptr("v-1")))
			Expect(p.BrandAudienceID).To(Equal(strptr("a-1")))

			// Persisted.
			resp = do("GET", "/api/posts/"+postID, nil)
			Expect(resp.StatusCode).To(Equal(200))
			var got models.Post
			Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
			Expect(got.BrandVoiceID).To(Equal(strptr("v-1")))
			Expect(got.BrandAudienceID).To(Equal(strptr("a-1")))
		})

		It("clears the refs when set to null", func() {
			Expect(do("PUT", "/api/posts/"+postID+"/brand", fiber.Map{"brand_voice_id": "v-1"}).StatusCode).To(Equal(200))
			resp := do("PUT", "/api/posts/"+postID+"/brand", fiber.Map{"brand_voice_id": nil, "brand_audience_id": nil})
			Expect(resp.StatusCode).To(Equal(200))
			var p models.Post
			Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
			Expect(p.BrandVoiceID).To(BeNil())
			Expect(p.BrandAudienceID).To(BeNil())
		})

		It("422s a voice/audience id not in the tenant", func() {
			Expect(do("PUT", "/api/posts/"+postID+"/brand", fiber.Map{"brand_voice_id": "nope"}).StatusCode).To(Equal(422))
			Expect(do("PUT", "/api/posts/"+postID+"/brand", fiber.Map{"brand_audience_id": "nope"}).StatusCode).To(Equal(422))
		})

		It("404s an unknown post", func() {
			Expect(do("PUT", "/api/posts/ghost/brand", fiber.Map{"brand_voice_id": "v-1"}).StatusCode).To(Equal(404))
		})
	})

	Describe("campaign brand fields", func() {
		It("sets and round-trips brand_voice_id/brand_audience_id, 422 on a foreign id", func() {
			body := fiber.Map{"name": "C", "campaign_type_id": seededCampaignTypeID, "brand_voice_id": "v-1", "brand_audience_id": "a-1"}
			resp := do("PUT", "/api/campaigns/"+campaignID, body)
			Expect(resp.StatusCode).To(Equal(200))
			var c models.Campaign
			Expect(json.NewDecoder(resp.Body).Decode(&c)).To(Succeed())
			Expect(c.BrandVoiceID).To(Equal(strptr("v-1")))
			Expect(c.BrandAudienceID).To(Equal(strptr("a-1")))

			bad := fiber.Map{"name": "C", "campaign_type_id": seededCampaignTypeID, "brand_voice_id": "nope"}
			Expect(do("PUT", "/api/campaigns/"+campaignID, bad).StatusCode).To(Equal(422))
		})
	})
})

// seededCampaignTypeID is a system campaign type seeded by the reference-data
// migration (same ids campaigns_test uses).
const seededCampaignTypeID = "gb"
