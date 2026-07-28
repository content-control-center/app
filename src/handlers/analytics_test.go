package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/database"
	"github.com/ogen-app/ogen/src/handlers"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

var _ = Describe("Analytics endpoints", Ordered, func() {
	var (
		app           *fiber.App
		db            *bun.DB
		authCookie    *http.Cookie
		campaignID    string
		userID        string
		postRepo      repository.PostRepository
		analyticsRepo repository.PostAnalyticsRepository
	)

	const linkedinSqid = "AXqWG7U2qnpt"

	BeforeAll(func() {
		db = mustOpenTestDBWithMigrations()
		// CON-125 Track B: post_analytics_snapshots lives in the analytics
		// migration set. In tests one physical DB holds both sets (the table
		// names don't collide), so the analytics repo is pointed at the same
		// pool after applying the analytics migrations.
		Expect(database.MigrateAnalytics(context.Background(), db)).To(Succeed())
	})

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
		settingRepo := repository.NewSettingRepository(db)
		tagRepo := repository.NewTagRepository(db)
		campaignTypeRepo := repository.NewCampaignTypeRepository(db)
		campaignRepo := repository.NewCampaignRepository(db, tagRepo, repository.NewPlatformRepository(db), campaignTypeRepo)
		postRepo = repository.NewPostRepository(db)
		analyticsRepo = repository.NewPostAnalyticsRepository(db)
		auth := handlers.RequireAuth(sessionRepo, testCookieName)

		handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(app)
		handlers.NewCampaignsHandler(campaignRepo, campaignTypeRepo, auth, nil, nil, nil, nil, nil).Register(app)

		ph := handlers.NewPostsHandler(postRepo, repository.NewPostVersionRepository(db),
			repository.NewPostAssistantMessageRepository(db), repository.NewPlatformRepository(db),
			repository.NewPostAttachmentRepository(db), auth, nil, nil)
		ph.SetAnalyticsRepo(analyticsRepo)
		ph.Register(app)
		handlers.NewAnalyticsHandler(analyticsRepo, auth).Register(app)

		// Auth user + login.
		createdUser := seedTenantUser(db, "Admin", "admin@example.com", "admin-password")
		userID = createdUser.ID

		loginBody, _ := json.Marshal(fiber.Map{"email": "admin@example.com", "password": "admin-password"})
		loginReq := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(loginBody))
		loginReq.Header.Set("Content-Type", "application/json")
		loginResp, err := app.Test(loginReq)
		Expect(err).NotTo(HaveOccurred())
		Expect(loginResp.StatusCode).To(Equal(fiber.StatusCreated))
		authCookie = loginResp.Cookies()[0]

		// Campaign for the seeded posts.
		cBody, _ := json.Marshal(fiber.Map{"name": "Analytics Campaign", "campaign_type_id": "Uk"})
		cReq := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(cBody))
		cReq.Header.Set("Content-Type", "application/json")
		cReq.AddCookie(authCookie)
		cResp, err := app.Test(cReq)
		Expect(err).NotTo(HaveOccurred())
		Expect(cResp.StatusCode).To(Equal(fiber.StatusCreated))
		var c models.Campaign
		Expect(json.NewDecoder(cResp.Body).Decode(&c)).To(Succeed())
		campaignID = c.ID
	})

	AfterEach(func() {
		ctx := tenantCtx()
		_, _ = db.NewDelete().TableExpr("post_analytics_snapshots").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("posts").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("campaigns").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(ctx)
	})

	seedPost := func(id, publisher, publisherPostID string, published time.Time) {
		p := &models.Post{
			ID:              id,
			CampaignID:      campaignID,
			PlatformID:      linkedinSqid,
			Title:           "Post " + id,
			Content:         "content " + id,
			MediaURLs:       models.StringSlice{},
			UsedAssetIDs:    models.StringSlice{},
			Status:          models.PostStatusPublished,
			Publisher:       publisher,
			PublisherPostID: publisherPostID,
			CTAType:         models.CTATypeNone,
			CreatedBy:       userID,
			PublishedAt:     &published,
		}
		Expect(postRepo.Create(tenantCtx(), p)).To(Succeed())
	}

	seedSnapshot := func(postID, pubPostID string, impressions, likes int, rate float64) {
		id, err := models.NewID()
		Expect(err).NotTo(HaveOccurred())
		Expect(analyticsRepo.Insert(tenantCtx(), &models.PostAnalytics{
			ID:              id,
			PostID:          postID,
			PublisherPostID: pubPostID,
			Publisher:       models.PublisherZernio,
			// Denormalised platform name (was joined from platforms.name).
			Platform:       "LinkedIn",
			Impressions:    impressions,
			Likes:          likes,
			EngagementRate: rate,
			PlatformAnalytics: models.PlatformAnalyticsList{
				{Platform: "linkedin", SyncStatus: "synced", Analytics: models.PostAnalyticsMetrics{Impressions: impressions, Likes: likes}},
			},
			SyncStatus: "synced",
			RawJSON:    "{}",
			OccurredAt: time.Now().UTC(),
		})).To(Succeed())
	}

	get := func(path string) *http.Response {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(authCookie)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	Describe("GET /api/posts/:id/analytics", func() {
		It("returns 404 for a missing post", func() {
			Expect(get("/api/posts/does-not-exist/analytics").StatusCode).To(Equal(404))
		})

		It("returns 409 not_published_via_publisher for a post without a publisher post id", func() {
			seedPost("p-draft", "", "", time.Now().UTC())
			resp := get("/api/posts/p-draft/analytics")
			Expect(resp.StatusCode).To(Equal(409))
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["code"]).To(Equal("not_published_via_publisher"))
		})

		It("returns 200 pending when the post has no snapshot yet", func() {
			seedPost("p-pending", models.PublisherZernio, "z-pending", time.Now().UTC())
			resp := get("/api/posts/p-pending/analytics")
			Expect(resp.StatusCode).To(Equal(200))
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["status"]).To(Equal("pending"))
			Expect(body["post_id"]).To(Equal("p-pending"))
		})

		It("returns 200 with the snapshot when covered", func() {
			seedPost("p1", models.PublisherZernio, "z-1", time.Now().UTC())
			seedSnapshot("p1", "z-1", 1234, 56, 0.07)
			resp := get("/api/posts/p1/analytics")
			Expect(resp.StatusCode).To(Equal(200))
			var body struct {
				PostID    string `json:"post_id"`
				Publisher string `json:"publisher"`
				Analytics struct {
					Impressions int `json:"impressions"`
					Likes       int `json:"likes"`
				} `json:"analytics"`
				PlatformAnalytics []struct {
					Platform   string `json:"platform"`
					SyncStatus string `json:"sync_status"`
				} `json:"platform_analytics"`
			}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body.PostID).To(Equal("p1"))
			Expect(body.Publisher).To(Equal("zernio"))
			Expect(body.Analytics.Impressions).To(Equal(1234))
			Expect(body.Analytics.Likes).To(Equal(56))
			Expect(body.PlatformAnalytics).To(HaveLen(1))
			Expect(body.PlatformAnalytics[0].Platform).To(Equal("linkedin"))
		})
	})

	Describe("GET /api/analytics/posts", func() {
		It("returns paged, sorted items plus overview, Zernio-only", func() {
			seedPost("p1", models.PublisherZernio, "z-1", time.Now().UTC().Add(-2*time.Hour))
			seedPost("p2", models.PublisherZernio, "z-2", time.Now().UTC().Add(-1*time.Hour))
			seedPost("p3", "", "", time.Now().UTC()) // non-publisher post, no snapshot
			seedSnapshot("p1", "z-1", 100, 10, 0.10)
			seedSnapshot("p2", "z-2", 300, 30, 0.20)

			resp := get("/api/analytics/posts?sort_by=impressions&order=desc&limit=50")
			Expect(resp.StatusCode).To(Equal(200))
			var body struct {
				Items []struct {
					PostID    string `json:"post_id"`
					Platform  string `json:"platform"`
					Analytics struct {
						Impressions int `json:"impressions"`
					} `json:"analytics"`
				} `json:"items"`
				Pagination struct {
					Page, Limit, Total, Pages int
				} `json:"pagination"`
				Overview struct {
					PostCount         int     `json:"post_count"`
					Impressions       int     `json:"impressions"`
					EngagementRateAvg float64 `json:"engagement_rate_avg"`
				} `json:"overview"`
			}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body.Items).To(HaveLen(2))
			Expect(body.Items[0].PostID).To(Equal("p2")) // impressions desc
			Expect(body.Items[0].Platform).To(Equal("LinkedIn"))
			Expect(body.Items[0].Analytics.Impressions).To(Equal(300))
			Expect(body.Pagination.Total).To(Equal(2))
			Expect(body.Overview.PostCount).To(Equal(2))
			Expect(body.Overview.Impressions).To(Equal(400))
		})

		It("rejects an unknown sort_by with 400", func() {
			Expect(get("/api/analytics/posts?sort_by=bogus").StatusCode).To(Equal(400))
		})

		It("rejects an unknown order with 400", func() {
			Expect(get("/api/analytics/posts?order=sideways").StatusCode).To(Equal(400))
		})
	})
})
