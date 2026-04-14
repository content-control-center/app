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

var _ = Describe("CampaignTypesHandler", Ordered, func() {
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
		campaignTypeRepo := repository.NewCampaignTypeRepository(db)
		auth := handlers.RequireAuth(sessionRepo, testCookieName)
		handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(app)
		handlers.NewCampaignTypesHandler(campaignTypeRepo, auth).Register(app)

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
		_, err := db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})

	// ── List ─────────────────────────────────────────────────────────────────

	Describe("GET /api/campaign_types", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/campaign_types", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns all 4 seeded campaign types", func() {
				req := httptest.NewRequest("GET", "/api/campaign_types", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var types []models.CampaignType
				Expect(json.NewDecoder(resp.Body).Decode(&types)).To(Succeed())
				Expect(types).To(HaveLen(4))

				names := make([]string, len(types))
				for i, ct := range types {
					names[i] = ct.Name
				}
				Expect(names).To(ConsistOf("awareness", "engagement", "conversion", "retention"))
			})

			It("returns phases for each campaign type", func() {
				req := httptest.NewRequest("GET", "/api/campaign_types", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())

				var types []models.CampaignType
				Expect(json.NewDecoder(resp.Body).Decode(&types)).To(Succeed())

				byName := make(map[string]models.CampaignType, len(types))
				for _, ct := range types {
					byName[ct.Name] = ct
				}

				Expect(byName["awareness"].Phases).To(HaveLen(2))
				Expect(byName["engagement"].Phases).To(HaveLen(3))
				Expect(byName["conversion"].Phases).To(HaveLen(3))
				Expect(byName["retention"].Phases).To(HaveLen(3))
			})

			It("returns phases ordered by sequence", func() {
				req := httptest.NewRequest("GET", "/api/campaign_types", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())

				var types []models.CampaignType
				Expect(json.NewDecoder(resp.Body).Decode(&types)).To(Succeed())

				byName := make(map[string]models.CampaignType, len(types))
				for _, ct := range types {
					byName[ct.Name] = ct
				}

				phases := byName["engagement"].Phases
				Expect(phases[0].Sequence).To(Equal(1))
				Expect(phases[1].Sequence).To(Equal(2))
				Expect(phases[2].Sequence).To(Equal(3))
				Expect(phases[0].Name).To(Equal("Activate"))
			})
		})
	})

	// ── Get ──────────────────────────────────────────────────────────────────

	Describe("GET /api/campaign_types/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/campaign_types/ct_awareness", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns the campaign type with phases", func() {
				req := httptest.NewRequest("GET", "/api/campaign_types/ct_awareness", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var ct models.CampaignType
				Expect(json.NewDecoder(resp.Body).Decode(&ct)).To(Succeed())
				Expect(ct.ID).To(Equal("ct_awareness"))
				Expect(ct.Name).To(Equal("awareness"))
				Expect(ct.Label).To(Equal("Awareness"))
				Expect(ct.Phases).To(HaveLen(2))
				Expect(ct.Phases[0].Name).To(Equal("Launch & Distribution"))
				Expect(ct.Phases[1].Name).To(Equal("Sustain & Optimize"))
			})

			It("returns 404 for an unknown id", func() {
				req := httptest.NewRequest("GET", "/api/campaign_types/ct_unknown", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})
})
