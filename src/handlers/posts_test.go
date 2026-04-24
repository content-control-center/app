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

	"github.com/content-control-center/app/src/handlers"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

var _ = Describe("PostsHandler", Ordered, func() {
	var (
		app        *fiber.App
		db         *bun.DB
		authCookie *http.Cookie
		campaignID string
	)

	BeforeAll(func() {
		db = mustOpenTestDBWithMigrations()
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
		pieceRepo := repository.NewAssetRepository(db, tagRepo, repository.NewAssetFileRepository(db))
		postRepo := repository.NewPostRepository(db)
		postVersionRepo := repository.NewPostVersionRepository(db)
		auth := handlers.RequireAuth(sessionRepo, testCookieName)
		handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(app)
		handlers.NewCampaignsHandler(campaignRepo, campaignTypeRepo, auth, nil).Register(app)
		handlers.NewAssetsHandler(pieceRepo, repository.NewAssetFileRepository(db), nil, auth, nil, nil).Register(app)
		postMessageRepo := repository.NewPostAssistantMessageRepository(db)
		handlers.NewPostsHandler(postRepo, postVersionRepo, postMessageRepo, auth, nil).Register(app)

		// Seed auth user and log in.
		body, _ := json.Marshal(fiber.Map{"name": "Admin", "email": "admin@example.com", "password": "admin-password"})
		req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

		loginBody, _ := json.Marshal(fiber.Map{"email": "admin@example.com", "password": "admin-password"})
		loginReq := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(loginBody))
		loginReq.Header.Set("Content-Type", "application/json")
		loginResp, err := app.Test(loginReq)
		Expect(err).NotTo(HaveOccurred())
		Expect(loginResp.StatusCode).To(Equal(fiber.StatusCreated))
		cookies := loginResp.Cookies()
		Expect(cookies).To(HaveLen(1))
		authCookie = cookies[0]

		// Seed a campaign used across tests.
		cBody, _ := json.Marshal(fiber.Map{"name": "Test Campaign", "campaign_type_id": "Uk"})
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
		_, err := db.NewDelete().TableExpr("post_assistant_messages").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("post_versions").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("posts").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("assets").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("campaigns").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})

	// helper: create a post via the API and return it
	createPost := func(title string, extraFields fiber.Map) models.Post {
		payload := fiber.Map{
			"campaign_id":        campaignID,
			"platform_id":        "AXqWG7U2qnpt",
			"platform_post_type": "text-post",
			"title":              title,
		}
		for k, v := range extraFields {
			payload[k] = v
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/posts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		var p models.Post
		Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
		return p
	}

	// ── List ─────────────────────────────────────────────────────────────────

	Describe("GET /api/posts", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/posts", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns an empty list when no posts exist", func() {
				req := httptest.NewRequest("GET", "/api/posts", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var posts []models.Post
				Expect(json.NewDecoder(resp.Body).Decode(&posts)).To(Succeed())
				Expect(posts).To(BeEmpty())
			})

			It("returns all posts with hydrated campaign and platform", func() {
				createPost("Post One", nil)
				createPost("Post Two", nil)

				req := httptest.NewRequest("GET", "/api/posts", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var posts []models.Post
				Expect(json.NewDecoder(resp.Body).Decode(&posts)).To(Succeed())
				Expect(posts).To(HaveLen(2))
				Expect(posts[0].Campaign).NotTo(BeNil())
				Expect(posts[0].Campaign.ID).To(Equal(campaignID))
				Expect(posts[0].Platform).NotTo(BeNil())
				Expect(posts[0].Platform.ID).To(Equal("AXqWG7U2qnpt"))
			})
		})
	})

	// ── List by campaign ─────────────────────────────────────────────────────

	Describe("GET /api/campaigns/:campaign_id/posts", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/campaigns/"+campaignID+"/posts", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns only posts belonging to the given campaign", func() {
				createPost("Campaign Post One", nil)
				createPost("Campaign Post Two", nil)

				req := httptest.NewRequest("GET", "/api/campaigns/"+campaignID+"/posts", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var posts []models.Post
				Expect(json.NewDecoder(resp.Body).Decode(&posts)).To(Succeed())
				Expect(posts).To(HaveLen(2))
				for _, p := range posts {
					Expect(p.CampaignID).To(Equal(campaignID))
					Expect(p.Campaign).NotTo(BeNil())
					Expect(p.Platform).NotTo(BeNil())
				}
			})

			It("returns an empty list for a campaign with no posts", func() {
				req := httptest.NewRequest("GET", "/api/campaigns/nonexistent/posts", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var posts []models.Post
				Expect(json.NewDecoder(resp.Body).Decode(&posts)).To(Succeed())
				Expect(posts).To(BeEmpty())
			})
		})
	})

	// ── Create ───────────────────────────────────────────────────────────────

	Describe("POST /api/posts", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{
					"campaign_id": campaignID, "platform_id": "AXqWG7U2qnpt",
					"platform_post_type": "text-post", "title": "Test",
				})
				req := httptest.NewRequest("POST", "/api/posts", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("creates a post with required fields and returns 201", func() {
				p := createPost("My First Post", nil)
				Expect(p.ID).NotTo(BeEmpty())
				Expect(p.Title).To(Equal("My First Post"))
				Expect(p.Status).To(Equal(models.PostStatusDraft))
				Expect(p.CTAType).To(Equal(models.CTATypeNone))
				Expect(p.CreatedBy).NotTo(BeEmpty())
				Expect(p.MediaURLs).To(BeEmpty())
				Expect(p.UsedAssets).To(BeEmpty())
			})

			It("creates a post with all optional fields", func() {
				p := createPost("Full Post", fiber.Map{
					"content":               "Post body text.",
					"status":                "ready_for_publish",
					"cta_type":              "link",
					"cta_url":               "https://example.com",
					"target_audience_notes": "Developers aged 25-40",
					"media_urls":            []string{"https://example.com/image.png"},
				})
				Expect(p.Status).To(Equal(models.PostStatusReadyForPublish))
				Expect(p.CTAType).To(Equal(models.CTATypeLink))
				Expect(p.CTAUrl).To(Equal("https://example.com"))
				Expect(p.MediaURLs).To(ConsistOf("https://example.com/image.png"))
				Expect(p.Content).To(Equal("Post body text."))
			})

			It("returns 400 when campaign_id is missing", func() {
				body, _ := json.Marshal(fiber.Map{
					"platform_id": "AXqWG7U2qnpt", "platform_post_type": "text-post", "title": "No Campaign",
				})
				req := httptest.NewRequest("POST", "/api/posts", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns 400 when platform_id is missing", func() {
				body, _ := json.Marshal(fiber.Map{
					"campaign_id": campaignID, "platform_post_type": "text-post", "title": "No Platform",
				})
				req := httptest.NewRequest("POST", "/api/posts", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("creates a post without a title", func() {
				body, _ := json.Marshal(fiber.Map{
					"campaign_id": campaignID, "platform_id": "AXqWG7U2qnpt", "platform_post_type": "text-post",
				})
				req := httptest.NewRequest("POST", "/api/posts", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

				var p models.Post
				Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
				Expect(p.Title).To(BeEmpty())
			})

			It("returns 400 when status is invalid", func() {
				body, _ := json.Marshal(fiber.Map{
					"campaign_id": campaignID, "platform_id": "AXqWG7U2qnpt",
					"platform_post_type": "text-post", "title": "Bad Status", "status": "unknown",
				})
				req := httptest.NewRequest("POST", "/api/posts", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns 400 when cta_type is invalid", func() {
				body, _ := json.Marshal(fiber.Map{
					"campaign_id": campaignID, "platform_id": "AXqWG7U2qnpt",
					"platform_post_type": "text-post", "title": "Bad CTA", "cta_type": "unknown",
				})
				req := httptest.NewRequest("POST", "/api/posts", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})
		})
	})

	// ── Get ──────────────────────────────────────────────────────────────────

	Describe("GET /api/posts/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/posts/someid", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns the post with hydrated campaign and platform", func() {
				p := createPost("Hydrated Post", nil)

				req := httptest.NewRequest("GET", "/api/posts/"+p.ID, nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Post
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.ID).To(Equal(p.ID))
				Expect(got.Campaign).NotTo(BeNil())
				Expect(got.Campaign.ID).To(Equal(campaignID))
				Expect(got.Campaign.Name).To(Equal("Test Campaign"))
				Expect(got.Platform).NotTo(BeNil())
				Expect(got.Platform.ID).To(Equal("AXqWG7U2qnpt"))
				Expect(got.Platform.Name).To(Equal("LinkedIn"))
				Expect(got.UsedAssets).To(BeEmpty())
			})

			It("returns the post with hydrated used_assets", func() {
				// Create a asset to reference.
				pBody, _ := json.Marshal(fiber.Map{"title": "Ref Asset", "content": `[]`})
				pReq := httptest.NewRequest("POST", "/api/content-bank/assets", bytes.NewReader(pBody))
				pReq.Header.Set("Content-Type", "application/json")
				pReq.AddCookie(authCookie)
				pResp, err := app.Test(pReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(pResp.StatusCode).To(Equal(fiber.StatusCreated))
				var asset models.Asset
				Expect(json.NewDecoder(pResp.Body).Decode(&asset)).To(Succeed())

				post := createPost("Post With Asset", fiber.Map{
					"used_asset_ids": []string{asset.ID},
				})

				req := httptest.NewRequest("GET", "/api/posts/"+post.ID, nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Post
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.UsedAssets).To(HaveLen(1))
				Expect(got.UsedAssets[0].ID).To(Equal(asset.ID))
				Expect(got.UsedAssets[0].Title).To(Equal("Ref Asset"))
			})

			It("returns 404 for an unknown id", func() {
				req := httptest.NewRequest("GET", "/api/posts/nonexistent", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})

	// ── Update ───────────────────────────────────────────────────────────────

	Describe("PUT /api/posts/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{
					"campaign_id": campaignID, "platform_id": "AXqWG7U2qnpt",
					"platform_post_type": "text-post", "title": "Updated",
				})
				req := httptest.NewRequest("PUT", "/api/posts/someid", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("updates the post and returns the hydrated resource", func() {
				p := createPost("Original Title", nil)

				body, _ := json.Marshal(fiber.Map{
					"campaign_id":        campaignID,
					"platform_id":        "AXqWG7U2qnpt",
					"platform_post_type": "image-post",
					"title":              "Updated Title",
					"status":             "ready_for_publish",
					"cta_type":           "button",
					"cta_url":            "https://example.com/cta",
				})
				req := httptest.NewRequest("PUT", "/api/posts/"+p.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Post
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.Title).To(Equal("Updated Title"))
				Expect(got.PlatformPostType).To(Equal("image-post"))
				Expect(got.Status).To(Equal(models.PostStatusReadyForPublish))
				Expect(got.CTAType).To(Equal(models.CTATypeButton))
				Expect(got.Campaign).NotTo(BeNil())
				Expect(got.Platform).NotTo(BeNil())
			})

			It("returns 404 for an unknown id", func() {
				body, _ := json.Marshal(fiber.Map{
					"campaign_id": campaignID, "platform_id": "AXqWG7U2qnpt",
					"platform_post_type": "text-post", "title": "Ghost",
				})
				req := httptest.NewRequest("PUT", "/api/posts/nonexistent", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})

			It("returns 400 for an invalid status transition", func() {
				p := createPost("Draft Post", nil) // status = draft

				body, _ := json.Marshal(fiber.Map{
					"campaign_id":        campaignID,
					"platform_id":        "AXqWG7U2qnpt",
					"platform_post_type": "text-post",
					"title":              "Skip To Published",
					"status":             "published",
				})
				req := httptest.NewRequest("PUT", "/api/posts/"+p.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("allows a valid status transition chain", func() {
				p := createPost("Lifecycle Post", nil) // draft

				// draft -> ready_for_publish
				body, _ := json.Marshal(fiber.Map{
					"campaign_id": campaignID, "platform_id": "AXqWG7U2qnpt",
					"platform_post_type": "text-post", "title": "Lifecycle Post",
					"status": "ready_for_publish",
				})
				req := httptest.NewRequest("PUT", "/api/posts/"+p.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				// ready_for_publish -> scheduled
				body, _ = json.Marshal(fiber.Map{
					"campaign_id": campaignID, "platform_id": "AXqWG7U2qnpt",
					"platform_post_type": "text-post", "title": "Lifecycle Post",
					"status": "scheduled",
				})
				req = httptest.NewRequest("PUT", "/api/posts/"+p.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err = app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				// scheduled -> published
				body, _ = json.Marshal(fiber.Map{
					"campaign_id": campaignID, "platform_id": "AXqWG7U2qnpt",
					"platform_post_type": "text-post", "title": "Lifecycle Post",
					"status": "published",
				})
				req = httptest.NewRequest("PUT", "/api/posts/"+p.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err = app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Post
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.Status).To(Equal(models.PostStatusPublished))
			})

			It("clears the title when omitted", func() {
				p := createPost("Has Title", nil)

				body, _ := json.Marshal(fiber.Map{
					"campaign_id": campaignID, "platform_id": "AXqWG7U2qnpt", "platform_post_type": "text-post",
				})
				req := httptest.NewRequest("PUT", "/api/posts/"+p.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Post
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.Title).To(BeEmpty())
			})
		})
	})

	// ── Delete ───────────────────────────────────────────────────────────────

	Describe("DELETE /api/posts/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("DELETE", "/api/posts/someid", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("deletes the post and returns 204", func() {
				p := createPost("To Delete", nil)

				req := httptest.NewRequest("DELETE", "/api/posts/"+p.ID, nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(204))
			})

			It("returns 404 for an unknown id", func() {
				req := httptest.NewRequest("DELETE", "/api/posts/nonexistent", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})

	// ── Assistant ────────────────────────────────────────────────────────────

	Describe("POST /api/posts/:id/assistant", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{"instruction": "make it shorter"})
				req := httptest.NewRequest("POST", "/api/posts/someid/assistant", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns 503 when assistant callback is nil", func() {
				p := createPost("Assist Me", nil)
				body, _ := json.Marshal(fiber.Map{"instruction": "make it shorter"})
				req := httptest.NewRequest("POST", "/api/posts/"+p.ID+"/assistant", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(503))
			})
		})
	})

	// ── Versions ─────────────────────────────────────────────────────────────

	Describe("GET /api/posts/:id/versions", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/posts/someid/versions", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns an empty list when no versions exist", func() {
				p := createPost("No Versions", nil)
				req := httptest.NewRequest("GET", "/api/posts/"+p.ID+"/versions", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var versions []models.PostVersion
				Expect(json.NewDecoder(resp.Body).Decode(&versions)).To(Succeed())
				Expect(versions).To(BeEmpty())
			})
		})
	})

	Describe("POST /api/posts/:id/versions", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{"note": "manual save"})
				req := httptest.NewRequest("POST", "/api/posts/someid/versions", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("creates a version and returns 201", func() {
				p := createPost("Versioned Post", fiber.Map{"content": "Original content"})

				body, _ := json.Marshal(fiber.Map{"note": "First manual save"})
				req := httptest.NewRequest("POST", "/api/posts/"+p.ID+"/versions", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var v models.PostVersion
				Expect(json.NewDecoder(resp.Body).Decode(&v)).To(Succeed())
				Expect(v.PostID).To(Equal(p.ID))
				Expect(v.VersionNumber).To(Equal(1))
				Expect(v.Content).To(Equal("Original content"))
				Expect(v.Note).To(Equal("First manual save"))
				Expect(v.Creator).To(Equal("user"))
			})

			It("increments version number", func() {
				p := createPost("Multi Version", fiber.Map{"content": "Some content"})

				// Version 1
				body, _ := json.Marshal(fiber.Map{"note": "v1"})
				req := httptest.NewRequest("POST", "/api/posts/"+p.ID+"/versions", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				// Version 2
				body, _ = json.Marshal(fiber.Map{"note": "v2"})
				req = httptest.NewRequest("POST", "/api/posts/"+p.ID+"/versions", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err = app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var v models.PostVersion
				Expect(json.NewDecoder(resp.Body).Decode(&v)).To(Succeed())
				Expect(v.VersionNumber).To(Equal(2))

				// List should show both
				listReq := httptest.NewRequest("GET", "/api/posts/"+p.ID+"/versions", nil)
				listReq.AddCookie(authCookie)
				listResp, err := app.Test(listReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(listResp.StatusCode).To(Equal(200))

				var versions []models.PostVersion
				Expect(json.NewDecoder(listResp.Body).Decode(&versions)).To(Succeed())
				Expect(versions).To(HaveLen(2))
				Expect(versions[0].VersionNumber).To(Equal(1))
				Expect(versions[1].VersionNumber).To(Equal(2))
			})

			It("returns 404 for an unknown post", func() {
				body, _ := json.Marshal(fiber.Map{"note": "ghost"})
				req := httptest.NewRequest("POST", "/api/posts/nonexistent/versions", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})
})
