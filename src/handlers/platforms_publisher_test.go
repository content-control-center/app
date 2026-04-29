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

	"github.com/content-control-center/app/src/handlers"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/publishers"
	pubzernio "github.com/content-control-center/app/src/publishers/zernio"
	"github.com/content-control-center/app/src/repository"
)

// PlatformsHandler enrichment tests (CON-63). Sibling file to
// platforms_test.go so the original tests stay focused on CRUD.
var _ = Describe("PlatformsHandler publishers enrichment", Ordered, func() {
	var (
		app          *fiber.App
		db           *bun.DB
		authCookie   *http.Cookie
		platformRepo repository.PlatformRepository
		accountRepo  repository.SocialAccountRepository
		settingRepo  repository.SettingRepository
	)

	BeforeAll(func() {
		db = mustOpenTestDBWithMigrations()
	})

	// setupApp wires a fresh Fiber app with the supplied publisher
	// slice, seeds a login user, and stores the session cookie.
	setupApp := func(pubs []publishers.Publisher) {
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
		settingRepo = repository.NewSettingRepository(db)
		platformRepo = repository.NewPlatformRepository(db)
		accountRepo = repository.NewSocialAccountRepository(db)
		auth := handlers.RequireAuth(sessionRepo, testCookieName)
		handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(app)
		handlers.NewPlatformsHandler(platformRepo, pubs, auth).Register(app)

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
	}

	BeforeEach(func() {
		// Ensure the linkedin platform row exists. Other tests in the
		// suite may delete seeded rows; INSERT...ON CONFLICT keeps this
		// test resilient to ordering.
		_, err := db.NewInsert().Model(&models.Platform{
			ID:   "linkedin",
			Name: "LinkedIn",
			PostTypes: models.PostTypeMap{
				"text-post":  "Text post",
				"image-post": "Image post",
				"carousel":   "Carousel",
				"video":      "Video",
				"article":    "Article",
			},
		}).On("CONFLICT (id) DO NOTHING").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		ctx := context.Background()
		_, _ = db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("social_accounts").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("settings").Where("key LIKE ?", "zernio.%").Exec(ctx)
	})

	// ── No publishers configured ────────────────────────────────────

	Describe("with no publishers", func() {
		BeforeEach(func() { setupApp(nil) })

		It("emits publishers: [] (not null) for every platform on List", func() {
			req := httptest.NewRequest("GET", "/api/platforms", nil)
			req.AddCookie(authCookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var body []map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body).NotTo(BeEmpty())
			for _, p := range body {
				pubs, ok := p["publishers"].([]any)
				Expect(ok).To(BeTrue(), "publishers should be a JSON array, not null")
				Expect(pubs).To(BeEmpty())
			}
		})

		It("emits publishers: [] on Get", func() {
			req := httptest.NewRequest("GET", "/api/platforms/linkedin", nil)
			req.AddCookie(authCookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			pubs, ok := body["publishers"].([]any)
			Expect(ok).To(BeTrue())
			Expect(pubs).To(BeEmpty())
		})
	})

	// ── Zernio publisher configured ─────────────────────────────────

	Describe("with Zernio configured", func() {
		buildZernioPublisher := func() publishers.Publisher {
			integ := pubzernio.NewIntegration(pubzernio.NewClient(pubzernio.StaticKey("test-key"), "http://stub", pubzernio.ClientOpts{Timeout: time.Second}))
			integ.SetState(pubzernio.StateOK)
			store := &settingStoreFromRepo{repo: settingRepo}
			Expect(store.Set(context.Background(), pubzernio.SettingProfileID, "p_test")).To(Succeed())
			return pubzernio.NewPublisher(integ, accountRepo, store)
		}

		It("with no accounts: connected=false, supported_post_types populated, accounts=[]", func() {
			setupApp([]publishers.Publisher{buildZernioPublisher()})

			req := httptest.NewRequest("GET", "/api/platforms/linkedin", nil)
			req.AddCookie(authCookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

			pubs := body["publishers"].([]any)
			Expect(pubs).To(HaveLen(1))
			z := pubs[0].(map[string]any)
			Expect(z["id"]).To(Equal("zernio"))
			Expect(z["name"]).To(Equal("Zernio"))
			Expect(z["state"]).To(Equal("ok"))
			Expect(z["connected"]).To(Equal(false))

			spt := z["supported_post_types"].([]any)
			Expect(spt).NotTo(BeEmpty())

			accts, ok := z["accounts"].([]any)
			Expect(ok).To(BeTrue(), "accounts should be a JSON array, not null")
			Expect(accts).To(BeEmpty())
		})

		It("matches platforms by name when local IDs differ from the allowlist (Sqid IDs)", func() {
			// Simulate a deployment where platforms.id holds a Sqid
			// rather than the seeded "instagram". The publisher
			// allowlist still uses "instagram" — the name fallback
			// should match it to the local row.
			ctx := context.Background()
			_, err := db.NewInsert().Model(&models.Platform{
				ID:   "rzgpTkARLH0L",
				Name: "Instagram",
				PostTypes: models.PostTypeMap{
					"image-post": "Image post",
					"reel":       "Reel",
				},
			}).On("CONFLICT (id) DO NOTHING").Exec(ctx)
			Expect(err).NotTo(HaveOccurred())

			setupApp([]publishers.Publisher{buildZernioPublisher()})

			now := time.Now().UTC()
			Expect(accountRepo.ApplyPlan(ctx, []models.SocialAccount{{
				ID:           "acc_ig",
				Platform:     "instagram",
				ProfileID:    "p_test",
				Username:     "ogen-team",
				DisplayName:  "Ogen",
				IsActive:     true,
				RawJSON:      "{}",
				ConnectedAt:  now,
				LastSyncedAt: now,
			}}, nil, now)).To(Succeed())

			req := httptest.NewRequest("GET", "/api/platforms/rzgpTkARLH0L", nil)
			req.AddCookie(authCookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			pubs := body["publishers"].([]any)
			Expect(pubs).To(HaveLen(1), "name fallback should attach Zernio publisher despite ID mismatch")
			z := pubs[0].(map[string]any)
			Expect(z["connected"]).To(Equal(true))
			Expect(z["accounts"].([]any)).To(HaveLen(1))
		})

		It("with a connected account: connected=true and the account in the response", func() {
			setupApp([]publishers.Publisher{buildZernioPublisher()})

			now := time.Now().UTC()
			Expect(accountRepo.ApplyPlan(context.Background(), []models.SocialAccount{{
				ID:           "acc1",
				Platform:     "linkedin", // Zernio's platform identifier
				ProfileID:    "p_test",
				Username:     "ogen-team",
				DisplayName:  "Ogen",
				AvatarURL:    "https://example.com/a.png",
				IsActive:     true,
				RawJSON:      "{}",
				ConnectedAt:  now,
				LastSyncedAt: now,
			}}, nil, now)).To(Succeed())

			req := httptest.NewRequest("GET", "/api/platforms/linkedin", nil)
			req.AddCookie(authCookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

			z := body["publishers"].([]any)[0].(map[string]any)
			Expect(z["connected"]).To(Equal(true))
			accts := z["accounts"].([]any)
			Expect(accts).To(HaveLen(1))
			acc := accts[0].(map[string]any)
			Expect(acc["id"]).To(Equal("acc1"))
			Expect(acc["username"]).To(Equal("ogen-team"))
			Expect(acc["display_name"]).To(Equal("Ogen"))
			Expect(acc["is_active"]).To(Equal(true))
		})
	})
})
