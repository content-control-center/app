package handlers_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/genkit/flows/content_plan"
	"github.com/ogen-app/ogen/src/handlers"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

var _ = Describe("CampaignsHandler", Ordered, func() {
	var (
		app        *fiber.App
		db         *bun.DB
		authCookie *http.Cookie
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
		platformRepo := repository.NewPlatformRepository(db)
		campaignTypeRepo := repository.NewCampaignTypeRepository(db)
		campaignRepo := repository.NewCampaignRepository(db, tagRepo, platformRepo, campaignTypeRepo)
		auth := handlers.RequireAuth(sessionRepo, testCookieName)
		handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(app)
		handlers.NewCampaignTypesHandler(campaignTypeRepo, auth).Register(app)
		handlers.NewCampaignsHandler(campaignRepo, campaignTypeRepo, auth, nil, nil).Register(app)
		handlers.NewTagsHandler(tagRepo, auth).Register(app)

		// Seed an auth user and log in
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
	})

	AfterEach(func() {
		_, err := db.NewDelete().TableExpr("campaigns").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("tags").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})

	// helper: create a campaign via the API and return it
	createCampaign := func(name string, campaignTypeID string) models.Campaign {
		body, _ := json.Marshal(fiber.Map{"name": name, "campaign_type_id": campaignTypeID})
		req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		var c models.Campaign
		Expect(json.NewDecoder(resp.Body).Decode(&c)).To(Succeed())
		return c
	}

	// ── List ─────────────────────────────────────────────────────────────────

	Describe("GET /api/campaigns", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/campaigns", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns an empty list when no campaigns exist", func() {
				req := httptest.NewRequest("GET", "/api/campaigns", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var campaigns []models.Campaign
				Expect(json.NewDecoder(resp.Body).Decode(&campaigns)).To(Succeed())
				Expect(campaigns).To(BeEmpty())
			})

			It("returns all campaigns", func() {
				createCampaign("Summer Push", "Uk")
				createCampaign("Q4 Retargeting", "Ef")

				req := httptest.NewRequest("GET", "/api/campaigns", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var campaigns []models.Campaign
				Expect(json.NewDecoder(resp.Body).Decode(&campaigns)).To(Succeed())
				Expect(campaigns).To(HaveLen(2))
			})
		})
	})

	// ── Create ───────────────────────────────────────────────────────────────

	Describe("POST /api/campaigns", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Test", "campaign_type_id": "Uk"})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("creates a campaign with required fields and returns 201", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Launch Campaign", "campaign_type_id": "gb"})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var c models.Campaign
				Expect(json.NewDecoder(resp.Body).Decode(&c)).To(Succeed())
				Expect(c.ID).NotTo(BeEmpty())
				Expect(c.Name).To(Equal("Launch Campaign"))
				Expect(c.CampaignTypeID).To(Equal("gb"))
				Expect(c.Status).To(Equal(models.StatusDraft))
				Expect(c.CreatedBy).NotTo(BeEmpty())
				Expect(c.AssetIDs).To(BeEmpty())
				Expect(c.TargetPlatforms).To(BeEmpty())
				Expect(c.Tags).To(BeEmpty())
			})

			It("creates a campaign with all optional fields", func() {
				count := 20
				body, _ := json.Marshal(fiber.Map{
					"name":             "Full Campaign",
					"campaign_type_id": "Ef",
					"description":      "Drive sign-ups",
					"target_persona":   "Tech-savvy millennials",
					"key_messages":     "Fast, reliable, affordable",
					"tone_guidelines":  "Friendly and professional",
					"use_assets":       true,
					"asset_ids":       []string{"abc", "def"},
					"target_platforms": []fiber.Map{
						{"id": "rzgpTkARLH0L", "post_types": []string{"image-post", "reel"}},
						{"id": "tiktok", "post_types": []string{"video"}},
					},
					"status":               "scheduled",
					"estimated_post_count": count,
					"budget":               1500.00,
					"currency":             "USD",
					"language":             "en",
					"tag_ids":              []string{},
				})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var c models.Campaign
				Expect(json.NewDecoder(resp.Body).Decode(&c)).To(Succeed())
				Expect(c.Status).To(Equal(models.StatusScheduled))
				Expect(c.UseAssets).To(BeTrue())
				Expect(c.AssetIDs).To(ConsistOf("abc", "def"))
				Expect(c.TagIDs).To(BeEmpty())
				Expect(c.EstimatedPostCount).NotTo(BeNil())
				Expect(*c.EstimatedPostCount).To(Equal(count))
				Expect(c.Budget).NotTo(BeNil())
				Expect(*c.Budget).To(BeNumerically("~", 1500.00, 0.01))
				Expect(c.Currency).To(Equal("USD"))
				Expect(c.Language).To(Equal("en"))
			})

			It("stores nil estimated_post_count when omitted", func() {
				body, _ := json.Marshal(fiber.Map{"name": "No Count", "campaign_type_id": "Uk"})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var c models.Campaign
				Expect(json.NewDecoder(resp.Body).Decode(&c)).To(Succeed())
				Expect(c.EstimatedPostCount).To(BeNil())
			})

			It("returns 400 when name is missing", func() {
				body, _ := json.Marshal(fiber.Map{"campaign_type_id": "Uk"})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns 400 when campaign_type_id is missing", func() {
				body, _ := json.Marshal(fiber.Map{"name": "No Type"})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns 400 when campaign_type_id is invalid", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Bad Type", "campaign_type_id": "nonexistent"})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns 400 when status is invalid", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Bad Status", "campaign_type_id": "Uk", "status": "unknown"})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})
		})
	})

	// ── Get ──────────────────────────────────────────────────────────────────

	Describe("GET /api/campaigns/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/campaigns/someid", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns the campaign with hydrated campaign_type and phases", func() {
				c := createCampaign("Brand Awareness", "Uk")

				req := httptest.NewRequest("GET", "/api/campaigns/"+c.ID, nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Campaign
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.ID).To(Equal(c.ID))
				Expect(got.Name).To(Equal("Brand Awareness"))
				Expect(got.CampaignType).NotTo(BeNil())
				Expect(got.CampaignType.Name).To(Equal("awareness"))
				Expect(got.CampaignType.Phases).To(HaveLen(2))
				Expect(got.CampaignType.Phases[0].Name).To(Equal("Launch & Distribution"))
			})

			It("returns hydrated platforms for known target_platforms", func() {
				body, _ := json.Marshal(fiber.Map{
					"name":             "Platform Hydration Test",
					"campaign_type_id": "Uk",
					"target_platforms": []fiber.Map{
						{"id": "rzgpTkARLH0L", "post_types": []string{"image-post"}},
						{"id": "tiktok", "post_types": []string{"video"}},
					},
				})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, _ := app.Test(req)
				var created models.Campaign
				json.NewDecoder(resp.Body).Decode(&created)

				req = httptest.NewRequest("GET", "/api/campaigns/"+created.ID, nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Campaign
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				// rzgpTkARLH0L is the seeded Instagram platform; "tiktok" has no match
				Expect(got.Platforms).To(HaveLen(1))
				Expect(got.Platforms[0].ID).To(Equal("rzgpTkARLH0L"))
				Expect(got.Platforms[0].Name).To(Equal("Instagram"))
			})

			It("returns 404 for an unknown id", func() {
				req := httptest.NewRequest("GET", "/api/campaigns/nonexistent", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})

	// ── Update ───────────────────────────────────────────────────────────────

	Describe("PUT /api/campaigns/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Updated", "campaign_type_id": "Vq"})
				req := httptest.NewRequest("PUT", "/api/campaigns/someid", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("updates the campaign and returns the updated resource", func() {
				c := createCampaign("Original Name", "Uk")

				count := 15
				body, _ := json.Marshal(fiber.Map{
					"name":                 "Updated Name",
					"campaign_type_id":     "Vq",
					"status":               "active",
					"language":             "fr",
					"estimated_post_count": count,
				})
				req := httptest.NewRequest("PUT", "/api/campaigns/"+c.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Campaign
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.Name).To(Equal("Updated Name"))
				Expect(got.CampaignTypeID).To(Equal("Vq"))
				Expect(got.Status).To(Equal(models.StatusActive))
				Expect(got.Language).To(Equal("fr"))
				Expect(got.EstimatedPostCount).NotTo(BeNil())
				Expect(*got.EstimatedPostCount).To(Equal(count))
			})

			It("returns 404 for an unknown id", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Ghost", "campaign_type_id": "Uk"})
				req := httptest.NewRequest("PUT", "/api/campaigns/nonexistent", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})

			It("returns 400 when name is missing", func() {
				c := createCampaign("Has Name", "gb")

				body, _ := json.Marshal(fiber.Map{"campaign_type_id": "Uk"})
				req := httptest.NewRequest("PUT", "/api/campaigns/"+c.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns 400 when campaign_type_id is invalid", func() {
				c := createCampaign("Valid", "gb")

				body, _ := json.Marshal(fiber.Map{"name": "Valid", "campaign_type_id": "nonexistent"})
				req := httptest.NewRequest("PUT", "/api/campaigns/"+c.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})
		})
	})

	// ── Tag hydration ────────────────────────────────────────────────────────

	Describe("tag hydration on GET /api/campaigns and /:id", func() {
		var tagID string

		BeforeEach(func() {
			tagBody, _ := json.Marshal(fiber.Map{"name": "Q4", "color": "#3498DB"})
			tagReq := httptest.NewRequest("POST", "/api/tags", bytes.NewReader(tagBody))
			tagReq.Header.Set("Content-Type", "application/json")
			tagReq.AddCookie(authCookie)
			tagResp, err := app.Test(tagReq)
			Expect(err).NotTo(HaveOccurred())
			Expect(tagResp.StatusCode).To(Equal(fiber.StatusCreated))
			var t map[string]any
			Expect(json.NewDecoder(tagResp.Body).Decode(&t)).To(Succeed())
			tagID = t["id"].(string)
		})

		It("returns tags populated on List", func() {
			body, _ := json.Marshal(fiber.Map{
				"name":             "Tagged Campaign",
				"campaign_type_id": "Uk",
				"tag_ids":          []string{tagID},
			})
			req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(authCookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

			listReq := httptest.NewRequest("GET", "/api/campaigns", nil)
			listReq.AddCookie(authCookie)
			listResp, err := app.Test(listReq)
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.StatusCode).To(Equal(200))

			var campaigns []models.Campaign
			Expect(json.NewDecoder(listResp.Body).Decode(&campaigns)).To(Succeed())
			Expect(campaigns).To(HaveLen(1))
			Expect(campaigns[0].TagIDs).To(ConsistOf(tagID))
			Expect(campaigns[0].Tags).To(HaveLen(1))
			Expect(campaigns[0].Tags[0].ID).To(Equal(tagID))
			Expect(campaigns[0].Tags[0].Name).To(Equal("Q4"))
		})

		It("returns tags populated on GetByID", func() {
			c := createCampaign("Untagged", "Uk")

			body, _ := json.Marshal(fiber.Map{
				"name":             "Untagged",
				"campaign_type_id": "Uk",
				"tag_ids":          []string{tagID},
			})
			req := httptest.NewRequest("PUT", "/api/campaigns/"+c.ID, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(authCookie)
			_, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())

			getReq := httptest.NewRequest("GET", "/api/campaigns/"+c.ID, nil)
			getReq.AddCookie(authCookie)
			getResp, err := app.Test(getReq)
			Expect(err).NotTo(HaveOccurred())
			Expect(getResp.StatusCode).To(Equal(200))

			var got models.Campaign
			Expect(json.NewDecoder(getResp.Body).Decode(&got)).To(Succeed())
			Expect(got.Tags).To(HaveLen(1))
			Expect(got.Tags[0].Name).To(Equal("Q4"))
		})

		It("returns empty tags array when no tag_ids set", func() {
			c := createCampaign("No Tags", "gb")

			getReq := httptest.NewRequest("GET", "/api/campaigns/"+c.ID, nil)
			getReq.AddCookie(authCookie)
			getResp, err := app.Test(getReq)
			Expect(err).NotTo(HaveOccurred())

			var got models.Campaign
			Expect(json.NewDecoder(getResp.Body).Decode(&got)).To(Succeed())
			Expect(got.Tags).NotTo(BeNil())
			Expect(got.Tags).To(BeEmpty())
		})
	})

	// ── GenerateDraft ────────────────────────────────────────────────────────

	Describe("POST /api/campaigns/:id/generate-draft", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("POST", "/api/campaigns/someid/generate-draft", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns 503 when generateDraft is nil", func() {
				// The test app is wired with generateDraft=nil.
				c := createCampaign("Draft Campaign", "Uk")
				req := httptest.NewRequest("POST", "/api/campaigns/"+c.ID+"/generate-draft", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(503))
			})

			It("returns 404 for an unknown campaign id", func() {
				// Wire a no-op generateDraft so we don't hit 503 first.
				appWithDraft := fiber.New(fiber.Config{
					ErrorHandler: func(c *fiber.Ctx, err error) error {
						code := fiber.StatusInternalServerError
						if e, ok := err.(*fiber.Error); ok {
							code = e.Code
						}
						return c.Status(code).JSON(fiber.Map{"error": err.Error()})
					},
				})
				campaignTypeRepo2 := repository.NewCampaignTypeRepository(db)
				campaignRepo2 := repository.NewCampaignRepository(db, repository.NewTagRepository(db), repository.NewPlatformRepository(db), campaignTypeRepo2)
				sessionRepo := repository.NewSessionRepository(db)
				settingRepo := repository.NewSettingRepository(db)
				userRepo := repository.NewUserRepository(db)
				auth2 := handlers.RequireAuth(sessionRepo, testCookieName)
				handlers.NewUsersHandler(userRepo, settingRepo, auth2).Register(appWithDraft)
				handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(appWithDraft)
				noop := func(_ context.Context, _ string, _ content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error) {
					return &content_plan.ContentPlanResponse{}, nil
				}
				handlers.NewCampaignsHandler(campaignRepo2, campaignTypeRepo2, auth2, noop, nil).Register(appWithDraft)

				req := httptest.NewRequest("POST", "/api/campaigns/nonexistent/generate-draft", nil)
				req.AddCookie(authCookie)
				resp, err := appWithDraft.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})

			It("returns 403 for another user's campaign", func() {
				// Create a second user whose campaign the first user should not access.
				appWithDraft := fiber.New(fiber.Config{
					ErrorHandler: func(c *fiber.Ctx, err error) error {
						code := fiber.StatusInternalServerError
						if e, ok := err.(*fiber.Error); ok {
							code = e.Code
						}
						return c.Status(code).JSON(fiber.Map{"error": err.Error()})
					},
				})
				campaignTypeRepo2 := repository.NewCampaignTypeRepository(db)
				campaignRepo2 := repository.NewCampaignRepository(db, repository.NewTagRepository(db), repository.NewPlatformRepository(db), campaignTypeRepo2)
				sessionRepo := repository.NewSessionRepository(db)
				settingRepo := repository.NewSettingRepository(db)
				userRepo := repository.NewUserRepository(db)
				auth2 := handlers.RequireAuth(sessionRepo, testCookieName)
				handlers.NewUsersHandler(userRepo, settingRepo, auth2).Register(appWithDraft)
				handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(appWithDraft)
				noop := func(_ context.Context, _ string, _ content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error) {
					return &content_plan.ContentPlanResponse{}, nil
				}
				handlers.NewCampaignsHandler(campaignRepo2, campaignTypeRepo2, auth2, noop, nil).Register(appWithDraft)

				// Register and log in as a second user.
				body, _ := json.Marshal(fiber.Map{"name": "Other", "email": "other@example.com", "password": "other-password"})
				regReq := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
				regReq.Header.Set("Content-Type", "application/json")
				regResp, err := appWithDraft.Test(regReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(regResp.StatusCode).To(Equal(fiber.StatusCreated))
				var otherUser models.User
				Expect(json.NewDecoder(regResp.Body).Decode(&otherUser)).To(Succeed())

				loginBody, _ := json.Marshal(fiber.Map{"email": "other@example.com", "password": "other-password"})
				loginReq := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(loginBody))
				loginReq.Header.Set("Content-Type", "application/json")
				loginResp, err := appWithDraft.Test(loginReq)
				Expect(err).NotTo(HaveOccurred())
				var otherCookie *http.Cookie
				for _, ck := range loginResp.Cookies() {
					otherCookie = ck
				}

				// Create a campaign as the second user.
				campBody, _ := json.Marshal(fiber.Map{"name": "Other's Campaign", "campaign_type_id": "Uk"})
				campReq := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(campBody))
				campReq.Header.Set("Content-Type", "application/json")
				campReq.AddCookie(otherCookie)
				campResp, err := appWithDraft.Test(campReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(campResp.StatusCode).To(Equal(fiber.StatusCreated))
				var otherCampaign models.Campaign
				Expect(json.NewDecoder(campResp.Body).Decode(&otherCampaign)).To(Succeed())

				// Try to generate draft as the first user (authCookie) — should get 403.
				draftReq := httptest.NewRequest("POST", "/api/campaigns/"+otherCampaign.ID+"/generate-draft", nil)
				draftReq.AddCookie(authCookie)
				draftResp, err := appWithDraft.Test(draftReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(draftResp.StatusCode).To(Equal(403))
			})

			It("streams step and complete SSE events on success", func() {
				appWithDraft := fiber.New(fiber.Config{
					ErrorHandler: func(c *fiber.Ctx, err error) error {
						code := fiber.StatusInternalServerError
						if e, ok := err.(*fiber.Error); ok {
							code = e.Code
						}
						return c.Status(code).JSON(fiber.Map{"error": err.Error()})
					},
				})
				campaignTypeRepo2 := repository.NewCampaignTypeRepository(db)
				campaignRepo2 := repository.NewCampaignRepository(db, repository.NewTagRepository(db), repository.NewPlatformRepository(db), campaignTypeRepo2)
				sessionRepo := repository.NewSessionRepository(db)
				settingRepo := repository.NewSettingRepository(db)
				userRepo := repository.NewUserRepository(db)
				auth2 := handlers.RequireAuth(sessionRepo, testCookieName)
				handlers.NewUsersHandler(userRepo, settingRepo, auth2).Register(appWithDraft)
				handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(appWithDraft)

				stub := func(_ context.Context, _ string, onEvent content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error) {
					onEvent(content_plan.SSEEventStep, content_plan.StepEventPayload{Step: "validateInput", Status: "done"})
					onEvent(content_plan.SSEEventPost, content_plan.PostEventPayload{
						Post:  content_plan.DraftPost{Title: "First Post", Body: "Body text", PlatformID: "linkedin", ContentType: "text-post", PublishDate: "2026-05-01"},
						Index: 0,
					})
					onEvent(content_plan.SSEEventStep, content_plan.StepEventPayload{Step: "generatePosts", Status: "done"})
					return &content_plan.ContentPlanResponse{CampaignID: "test"}, nil
				}
				handlers.NewCampaignsHandler(campaignRepo2, campaignTypeRepo2, auth2, stub, nil).Register(appWithDraft)

				// Seed user/session for appWithDraft.
				body, _ := json.Marshal(fiber.Map{"name": "SSE User", "email": "sse@example.com", "password": "sse-password"})
				regReq := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
				regReq.Header.Set("Content-Type", "application/json")
				_, err := appWithDraft.Test(regReq)
				Expect(err).NotTo(HaveOccurred())
				loginBody, _ := json.Marshal(fiber.Map{"email": "sse@example.com", "password": "sse-password"})
				loginReq := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(loginBody))
				loginReq.Header.Set("Content-Type", "application/json")
				loginResp, err := appWithDraft.Test(loginReq)
				Expect(err).NotTo(HaveOccurred())
				var sseCookie *http.Cookie
				for _, ck := range loginResp.Cookies() {
					sseCookie = ck
				}

				// Create campaign.
				campBody, _ := json.Marshal(fiber.Map{"name": "SSE Campaign", "campaign_type_id": "Uk"})
				campReq := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(campBody))
				campReq.Header.Set("Content-Type", "application/json")
				campReq.AddCookie(sseCookie)
				campResp, err := appWithDraft.Test(campReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(campResp.StatusCode).To(Equal(fiber.StatusCreated))
				var camp models.Campaign
				Expect(json.NewDecoder(campResp.Body).Decode(&camp)).To(Succeed())

				// Call generate-draft.
				draftReq := httptest.NewRequest("POST", "/api/campaigns/"+camp.ID+"/generate-draft", nil)
				draftReq.AddCookie(sseCookie)
				draftResp, err := appWithDraft.Test(draftReq, 10000)
				Expect(err).NotTo(HaveOccurred())
				Expect(draftResp.StatusCode).To(Equal(200))
				Expect(draftResp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

				// Parse SSE lines.
				type sseEvent struct{ event, data string }
				var events []sseEvent
				scanner := bufio.NewScanner(draftResp.Body)
				var curEvent, curData string
				for scanner.Scan() {
					line := scanner.Text()
					switch {
					case strings.HasPrefix(line, "event: "):
						curEvent = strings.TrimPrefix(line, "event: ")
					case strings.HasPrefix(line, "data: "):
						curData = strings.TrimPrefix(line, "data: ")
					case line == "":
						if curEvent != "" {
							events = append(events, sseEvent{curEvent, curData})
						}
						curEvent, curData = "", ""
					}
				}

				Expect(events).To(HaveLen(4)) // 2 step + 1 post + 1 complete
				Expect(events[0].event).To(Equal("step"))
				Expect(events[1].event).To(Equal("post"))
				Expect(events[2].event).To(Equal("step"))
				Expect(events[3].event).To(Equal("complete"))

				var postPayload content_plan.PostEventPayload
				Expect(json.Unmarshal([]byte(events[1].data), &postPayload)).To(Succeed())
				Expect(postPayload.Index).To(Equal(0))
				Expect(postPayload.Post.Title).To(Equal("First Post"))

				var completePayload content_plan.ContentPlanResponse
				Expect(json.Unmarshal([]byte(events[3].data), &completePayload)).To(Succeed())
				Expect(completePayload.CampaignID).To(Equal("test"))
			})

			It("streams an error SSE event on failure", func() {
				appWithDraft := fiber.New(fiber.Config{
					ErrorHandler: func(c *fiber.Ctx, err error) error {
						code := fiber.StatusInternalServerError
						if e, ok := err.(*fiber.Error); ok {
							code = e.Code
						}
						return c.Status(code).JSON(fiber.Map{"error": err.Error()})
					},
				})
				campaignTypeRepo2 := repository.NewCampaignTypeRepository(db)
				campaignRepo2 := repository.NewCampaignRepository(db, repository.NewTagRepository(db), repository.NewPlatformRepository(db), campaignTypeRepo2)
				sessionRepo := repository.NewSessionRepository(db)
				settingRepo := repository.NewSettingRepository(db)
				userRepo := repository.NewUserRepository(db)
				auth2 := handlers.RequireAuth(sessionRepo, testCookieName)
				handlers.NewUsersHandler(userRepo, settingRepo, auth2).Register(appWithDraft)
				handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(appWithDraft)

				stub := func(_ context.Context, _ string, _ content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error) {
					return nil, &content_plan.ValidationError{Msg: "missing required fields"}
				}
				handlers.NewCampaignsHandler(campaignRepo2, campaignTypeRepo2, auth2, stub, nil).Register(appWithDraft)

				// Seed user/session.
				body, _ := json.Marshal(fiber.Map{"name": "Err User", "email": "err@example.com", "password": "err-password"})
				regReq := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
				regReq.Header.Set("Content-Type", "application/json")
				_, err := appWithDraft.Test(regReq)
				Expect(err).NotTo(HaveOccurred())
				loginBody, _ := json.Marshal(fiber.Map{"email": "err@example.com", "password": "err-password"})
				loginReq := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(loginBody))
				loginReq.Header.Set("Content-Type", "application/json")
				loginResp, err := appWithDraft.Test(loginReq)
				Expect(err).NotTo(HaveOccurred())
				var errCookie *http.Cookie
				for _, ck := range loginResp.Cookies() {
					errCookie = ck
				}

				campBody, _ := json.Marshal(fiber.Map{"name": "Err Campaign", "campaign_type_id": "Uk"})
				campReq := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(campBody))
				campReq.Header.Set("Content-Type", "application/json")
				campReq.AddCookie(errCookie)
				campResp, err := appWithDraft.Test(campReq)
				Expect(err).NotTo(HaveOccurred())
				var camp models.Campaign
				Expect(json.NewDecoder(campResp.Body).Decode(&camp)).To(Succeed())

				draftReq := httptest.NewRequest("POST", "/api/campaigns/"+camp.ID+"/generate-draft", nil)
				draftReq.AddCookie(errCookie)
				draftResp, err := appWithDraft.Test(draftReq, 10000)
				Expect(err).NotTo(HaveOccurred())
				Expect(draftResp.StatusCode).To(Equal(200))

				scanner := bufio.NewScanner(draftResp.Body)
				var eventType, eventData string
				for scanner.Scan() {
					line := scanner.Text()
					if strings.HasPrefix(line, "event: ") {
						eventType = strings.TrimPrefix(line, "event: ")
					} else if strings.HasPrefix(line, "data: ") {
						eventData = strings.TrimPrefix(line, "data: ")
					}
				}
				Expect(eventType).To(Equal("error"))
				var errPayload content_plan.ErrorEventPayload
				Expect(json.Unmarshal([]byte(eventData), &errPayload)).To(Succeed())
				Expect(errPayload.Code).To(Equal(400))
				Expect(errPayload.Message).To(Equal("missing required fields"))
			})
		})
	})

	// ── Delete ───────────────────────────────────────────────────────────────

	Describe("DELETE /api/campaigns/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("DELETE", "/api/campaigns/someid", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("deletes the campaign and returns 204", func() {
				c := createCampaign("To Delete", "Ef")

				req := httptest.NewRequest("DELETE", "/api/campaigns/"+c.ID, nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(204))
			})

			It("returns 404 for an unknown id", func() {
				req := httptest.NewRequest("DELETE", "/api/campaigns/nonexistent", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})
})
