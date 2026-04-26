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

var _ = Describe("PlatformsHandler", Ordered, func() {
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
		platformRepo := repository.NewPlatformRepository(db)
		auth := handlers.RequireAuth(sessionRepo, testCookieName)
		handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(app)
		handlers.NewPlatformsHandler(platformRepo, nil, auth).Register(app)

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
		_, err := db.NewDelete().TableExpr("platforms").Where("id NOT IN ('AXqWG7U2qnpt','8S8bWQTG6qD','zBU1zqVICGfk','81mUCmc2xsKd','pQ4yxT3SuE57','rzgpTkARLH0L')").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})

	// helper: create a platform via the API and return it
	createPlatform := func(name string, postTypes models.PostTypeMap, cadence, constraints string) models.Platform {
		body, _ := json.Marshal(fiber.Map{"name": name, "post_types": postTypes, "cadence": cadence, "constraints": constraints})
		req := httptest.NewRequest("POST", "/api/platforms", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		var p models.Platform
		Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
		return p
	}

	// ── List ─────────────────────────────────────────────────────────────────

	Describe("GET /api/platforms", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/platforms", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns the seeded platforms", func() {
				req := httptest.NewRequest("GET", "/api/platforms", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var platforms []models.Platform
				Expect(json.NewDecoder(resp.Body).Decode(&platforms)).To(Succeed())
				Expect(platforms).To(HaveLen(6))
			})

			It("includes newly created platforms", func() {
				createPlatform("TikTok", models.PostTypeMap{"video": "Video", "live": "Live"}, "", "")

				req := httptest.NewRequest("GET", "/api/platforms", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var platforms []models.Platform
				Expect(json.NewDecoder(resp.Body).Decode(&platforms)).To(Succeed())
				Expect(platforms).To(HaveLen(7))
			})
		})
	})

	// ── Create ───────────────────────────────────────────────────────────────

	Describe("POST /api/platforms", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{"name": "TikTok"})
				req := httptest.NewRequest("POST", "/api/platforms", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("creates a platform with post_types and returns 201", func() {
				body, _ := json.Marshal(fiber.Map{
					"name":       "TikTok",
					"post_types": fiber.Map{"video": "Video", "live": "Live", "story": "Story"},
				})
				req := httptest.NewRequest("POST", "/api/platforms", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var p models.Platform
				Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
				Expect(p.ID).NotTo(BeEmpty())
				Expect(p.Name).To(Equal("TikTok"))
				Expect(p.PostTypes).To(HaveKeyWithValue("video", "Video"))
				Expect(p.PostTypes).To(HaveKeyWithValue("live", "Live"))
				Expect(p.PostTypes).To(HaveKeyWithValue("story", "Story"))
			})

			It("creates a platform with cadence and constraints and returns them", func() {
				body, _ := json.Marshal(fiber.Map{
					"name":        "TikTok",
					"post_types":  fiber.Map{"video": "Video"},
					"cadence":     "1–2 videos per day",
					"constraints": "videos up to 10 min; captions up to 2200 chars",
				})
				req := httptest.NewRequest("POST", "/api/platforms", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var p models.Platform
				Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
				Expect(p.Cadence).To(Equal("1–2 videos per day"))
				Expect(p.Constraints).To(Equal("videos up to 10 min; captions up to 2200 chars"))
			})

			It("creates a platform without post_types and returns an empty map", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Pinterest"})
				req := httptest.NewRequest("POST", "/api/platforms", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var p models.Platform
				Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
				Expect(p.PostTypes).NotTo(BeNil())
				Expect(p.PostTypes).To(BeEmpty())
			})

			It("returns 400 when name is missing", func() {
				body, _ := json.Marshal(fiber.Map{"post_types": fiber.Map{}})
				req := httptest.NewRequest("POST", "/api/platforms", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})
		})
	})

	// ── Get ──────────────────────────────────────────────────────────────────

	Describe("GET /api/platforms/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/platforms/AXqWG7U2qnpt", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns the seeded LinkedIn platform with all post types, cadence, and constraints", func() {
				req := httptest.NewRequest("GET", "/api/platforms/AXqWG7U2qnpt", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var p models.Platform
				Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
				Expect(p.ID).To(Equal("AXqWG7U2qnpt"))
				Expect(p.Name).To(Equal("LinkedIn"))
				Expect(p.PostTypes).To(HaveKey("text-post"))
				Expect(p.PostTypes).To(HaveKey("article"))
				Expect(p.PostTypes).To(HaveLen(9))
				Expect(p.Cadence).To(Equal("1–2 posts per week"))
				Expect(p.Constraints).To(ContainSubstring("3000 chars"))
			})

			It("returns a created platform", func() {
				p := createPlatform("TikTok", models.PostTypeMap{"video": "Video"}, "", "")

				req := httptest.NewRequest("GET", "/api/platforms/"+p.ID, nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Platform
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.ID).To(Equal(p.ID))
				Expect(got.Name).To(Equal("TikTok"))
			})

			It("returns 404 for an unknown id", func() {
				req := httptest.NewRequest("GET", "/api/platforms/nonexistent", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})

	// ── Update ───────────────────────────────────────────────────────────────

	Describe("PUT /api/platforms/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Updated"})
				req := httptest.NewRequest("PUT", "/api/platforms/linkedin", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("updates name and post_types and returns the updated resource", func() {
				p := createPlatform("Old Name", models.PostTypeMap{"video": "Video"}, "", "")

				body, _ := json.Marshal(fiber.Map{
					"name":       "New Name",
					"post_types": fiber.Map{"video": "Video", "short": "Short"},
				})
				req := httptest.NewRequest("PUT", "/api/platforms/"+p.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Platform
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.Name).To(Equal("New Name"))
				Expect(got.PostTypes).To(HaveKeyWithValue("short", "Short"))
			})

			It("updates cadence and constraints and persists them", func() {
				p := createPlatform("TikTok", models.PostTypeMap{"video": "Video"}, "1–2 videos per day", "videos up to 10 min")

				body, _ := json.Marshal(fiber.Map{
					"name":        "TikTok",
					"post_types":  fiber.Map{"video": "Video"},
					"cadence":     "3–5 videos per day",
					"constraints": "videos up to 3 min; captions up to 150 chars",
				})
				req := httptest.NewRequest("PUT", "/api/platforms/"+p.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Platform
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.Cadence).To(Equal("3–5 videos per day"))
				Expect(got.Constraints).To(Equal("videos up to 3 min; captions up to 150 chars"))
			})

			It("returns 404 for an unknown id", func() {
				body, _ := json.Marshal(fiber.Map{"name": "Ghost"})
				req := httptest.NewRequest("PUT", "/api/platforms/nonexistent", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})

			It("returns 400 when name is missing", func() {
				p := createPlatform("Has Name", models.PostTypeMap{}, "", "")

				body, _ := json.Marshal(fiber.Map{"post_types": fiber.Map{}})
				req := httptest.NewRequest("PUT", "/api/platforms/"+p.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})
		})
	})

	// ── Delete ───────────────────────────────────────────────────────────────

	Describe("DELETE /api/platforms/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("DELETE", "/api/platforms/linkedin", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("deletes a created platform and returns 204", func() {
				p := createPlatform("To Delete", models.PostTypeMap{}, "", "")

				req := httptest.NewRequest("DELETE", "/api/platforms/"+p.ID, nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(204))
			})

			It("returns 404 for an unknown id", func() {
				req := httptest.NewRequest("DELETE", "/api/platforms/nonexistent", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})
})
