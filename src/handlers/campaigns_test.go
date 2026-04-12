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
		campaignRepo := repository.NewCampaignRepository(db, tagRepo)
		auth := handlers.RequireAuth(sessionRepo, testCookieName)
		handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(app)
		handlers.NewCampaignsHandler(campaignRepo, auth).Register(app)
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
	createCampaign := func(name string, objective models.CampaignObjective) models.Campaign {
		body, _ := json.Marshal(fiber.Map{"name": name, "objective": objective})
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
				createCampaign("Summer Push", models.ObjectiveAwareness)
				createCampaign("Q4 Retargeting", models.ObjectiveConversion)

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
				body, _ := json.Marshal(fiber.Map{"name": "Test", "objective": "awareness"})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("creates a campaign with required fields and returns 201", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Launch Campaign", "objective": "engagement"})
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
				Expect(c.Objective).To(Equal(models.ObjectiveEngagement))
				Expect(c.Status).To(Equal(models.StatusDraft))
				Expect(c.CreatedBy).NotTo(BeEmpty())
				Expect(c.PiecesIDs).To(BeEmpty())
				Expect(c.TargetPlatformIDs).To(BeEmpty())
				Expect(c.Tags).To(BeEmpty())
			})

			It("creates a campaign with all optional fields", func() {
				count := 20
				body, _ := json.Marshal(fiber.Map{
					"name":                 "Full Campaign",
					"objective":            "conversion",
					"description":          "Drive sign-ups",
					"target_persona":       "Tech-savvy millennials",
					"key_messages":         "Fast, reliable, affordable",
					"tone_guidelines":      "Friendly and professional",
					"use_pieces":           true,
					"pieces_ids":           []string{"abc", "def"},
					"target_platform_ids":  []string{"instagram", "tiktok"},
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
				Expect(c.UsePieces).To(BeTrue())
				Expect(c.PiecesIDs).To(ConsistOf("abc", "def"))
				Expect(c.TagIDs).To(BeEmpty())
				Expect(c.EstimatedPostCount).NotTo(BeNil())
				Expect(*c.EstimatedPostCount).To(Equal(count))
				Expect(c.Budget).NotTo(BeNil())
				Expect(*c.Budget).To(BeNumerically("~", 1500.00, 0.01))
				Expect(c.Currency).To(Equal("USD"))
				Expect(c.Language).To(Equal("en"))
			})

			It("stores nil estimated_post_count when omitted", func() {
				body, _ := json.Marshal(fiber.Map{"name": "No Count", "objective": "awareness"})
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
				body, _ := json.Marshal(fiber.Map{"objective": "awareness"})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns 400 when objective is missing", func() {
				body, _ := json.Marshal(fiber.Map{"name": "No Objective"})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns 400 when objective is invalid", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Bad Objective", "objective": "unknown"})
				req := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns 400 when status is invalid", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Bad Status", "objective": "awareness", "status": "unknown"})
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
			It("returns the campaign", func() {
				c := createCampaign("Brand Awareness", models.ObjectiveAwareness)

				req := httptest.NewRequest("GET", "/api/campaigns/"+c.ID, nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Campaign
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.ID).To(Equal(c.ID))
				Expect(got.Name).To(Equal("Brand Awareness"))
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
				body, _ := json.Marshal(fiber.Map{"name": "Updated", "objective": "retention"})
				req := httptest.NewRequest("PUT", "/api/campaigns/someid", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("updates the campaign and returns the updated resource", func() {
				c := createCampaign("Original Name", models.ObjectiveAwareness)

				count := 15
				body, _ := json.Marshal(fiber.Map{
					"name":                 "Updated Name",
					"objective":            "retention",
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
				Expect(got.Objective).To(Equal(models.ObjectiveRetention))
				Expect(got.Status).To(Equal(models.StatusActive))
				Expect(got.Language).To(Equal("fr"))
				Expect(got.EstimatedPostCount).NotTo(BeNil())
				Expect(*got.EstimatedPostCount).To(Equal(count))
			})

			It("returns 404 for an unknown id", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Ghost", "objective": "awareness"})
				req := httptest.NewRequest("PUT", "/api/campaigns/nonexistent", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})

			It("returns 400 when name is missing", func() {
				c := createCampaign("Has Name", models.ObjectiveEngagement)

				body, _ := json.Marshal(fiber.Map{"objective": "awareness"})
				req := httptest.NewRequest("PUT", "/api/campaigns/"+c.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns 400 when objective is invalid", func() {
				c := createCampaign("Valid", models.ObjectiveEngagement)

				body, _ := json.Marshal(fiber.Map{"name": "Valid", "objective": "bogus"})
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
				"name":      "Tagged Campaign",
				"objective": "awareness",
				"tag_ids":   []string{tagID},
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
			c := createCampaign("Untagged", models.ObjectiveAwareness)

			body, _ := json.Marshal(fiber.Map{
				"name":      "Untagged",
				"objective": "awareness",
				"tag_ids":   []string{tagID},
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
			c := createCampaign("No Tags", models.ObjectiveEngagement)

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
				c := createCampaign("To Delete", models.ObjectiveConversion)

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
