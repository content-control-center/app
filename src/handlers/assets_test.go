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

	"github.com/ogen-app/ogen/src/handlers"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

var _ = Describe("AssetsHandler", Ordered, func() {
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
		pieceRepo := repository.NewAssetRepository(db, tagRepo, repository.NewAssetFileRepository(db))
		auth := handlers.RequireAuth(sessionRepo, testCookieName)
		handlers.NewUsersHandler(db, userRepo, repository.NewAccountRepository(db), settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, repository.NewAccountRepository(db), sessionRepo, testCookieName, false).Register(app)
		handlers.NewAssetsHandler(pieceRepo, repository.NewAssetFileRepository(db), nil, nil, nil, auth, nil).Register(app)
		handlers.NewTagsHandler(tagRepo, auth).Register(app)

		// Seed an auth user and log in
		seedTenantUser(db, "Admin", "admin@example.com", "admin-password")

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
		_, err := db.NewDelete().TableExpr("assets").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("tags").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(context.Background())
		_, err = db.NewDelete().TableExpr("accounts").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})

	// helper: create a asset via the API and return it
	createPiece := func(title, content string) models.Asset {
		body, _ := json.Marshal(fiber.Map{"title": title, "content": content})
		req := httptest.NewRequest("POST", "/api/content-bank/assets", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		var p models.Asset
		Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
		return p
	}

	// ── List ─────────────────────────────────────────────────────────────────

	Describe("GET /api/content-bank/assets", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/content-bank/assets", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns an empty list when no assets exist", func() {
				req := httptest.NewRequest("GET", "/api/content-bank/assets", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var assets []models.Asset
				Expect(json.NewDecoder(resp.Body).Decode(&assets)).To(Succeed())
				Expect(assets).To(BeEmpty())
			})

			It("returns all assets", func() {
				createPiece("First", `[{"type":"paragraph"}]`)
				createPiece("Second", `[{"type":"paragraph"}]`)

				req := httptest.NewRequest("GET", "/api/content-bank/assets", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var assets []models.Asset
				Expect(json.NewDecoder(resp.Body).Decode(&assets)).To(Succeed())
				Expect(assets).To(HaveLen(2))
			})
		})
	})

	// ── Create ───────────────────────────────────────────────────────────────

	Describe("POST /api/content-bank/assets", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{"title": "Test", "content": `[]`})
				req := httptest.NewRequest("POST", "/api/content-bank/assets", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("creates a asset and returns 201 with the created_by from the session", func() {
				body, _ := json.Marshal(fiber.Map{"title": "My Asset", "content": `[{"type":"paragraph"}]`})
				req := httptest.NewRequest("POST", "/api/content-bank/assets", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var p models.Asset
				Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
				Expect(p.ID).NotTo(BeEmpty())
				Expect(p.Title).To(Equal("My Asset"))
				Expect(p.Content).To(Equal(`[{"type":"paragraph"}]`))
				Expect(p.CreatedBy).NotTo(BeEmpty())
			})

			It("returns 400 when title is missing", func() {
				body, _ := json.Marshal(fiber.Map{"content": `[]`})
				req := httptest.NewRequest("POST", "/api/content-bank/assets", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns 400 when content is missing", func() {
				body, _ := json.Marshal(fiber.Map{"title": "No Content"})
				req := httptest.NewRequest("POST", "/api/content-bank/assets", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})
		})
	})

	// ── Get ──────────────────────────────────────────────────────────────────

	Describe("GET /api/content-bank/assets/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/content-bank/assets/someid", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns the asset", func() {
				p := createPiece("Hello", `[{"type":"paragraph"}]`)

				req := httptest.NewRequest("GET", "/api/content-bank/assets/"+p.ID, nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Asset
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.ID).To(Equal(p.ID))
				Expect(got.Title).To(Equal("Hello"))
			})

			It("returns 404 for an unknown id", func() {
				req := httptest.NewRequest("GET", "/api/content-bank/assets/nonexistent", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})

	// ── Update ───────────────────────────────────────────────────────────────

	Describe("PUT /api/content-bank/assets/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{"title": "Updated", "content": `[]`})
				req := httptest.NewRequest("PUT", "/api/content-bank/assets/someid", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("updates the asset and returns the updated resource", func() {
				p := createPiece("Original", `[{"type":"paragraph"}]`)

				body, _ := json.Marshal(fiber.Map{"title": "Updated", "content": `[{"type":"heading"}]`})
				req := httptest.NewRequest("PUT", "/api/content-bank/assets/"+p.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.Asset
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.Title).To(Equal("Updated"))
				Expect(got.Content).To(Equal(`[{"type":"heading"}]`))
			})

			It("returns 404 for an unknown id", func() {
				body, _ := json.Marshal(fiber.Map{"title": "Updated", "content": `[]`})
				req := httptest.NewRequest("PUT", "/api/content-bank/assets/nonexistent", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})

			It("returns 400 when body is invalid", func() {
				p := createPiece("Original", `[]`)

				body, _ := json.Marshal(fiber.Map{"title": "No Content Field"})
				req := httptest.NewRequest("PUT", "/api/content-bank/assets/"+p.ID, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})
		})
	})

	// ── Tag hydration ────────────────────────────────────────────────────────

	Describe("tag hydration on GET /api/content-bank/assets and /:id", func() {
		var tagID string

		BeforeEach(func() {
			tagBody, _ := json.Marshal(fiber.Map{"name": "Featured", "color": "#FF5733"})
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
				"title":   "Tagged Asset",
				"content": `[{"type":"paragraph"}]`,
				"tag_ids": []string{tagID},
			})
			req := httptest.NewRequest("POST", "/api/content-bank/assets", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(authCookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

			listReq := httptest.NewRequest("GET", "/api/content-bank/assets", nil)
			listReq.AddCookie(authCookie)
			listResp, err := app.Test(listReq)
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp.StatusCode).To(Equal(200))

			var assets []models.Asset
			Expect(json.NewDecoder(listResp.Body).Decode(&assets)).To(Succeed())
			Expect(assets).To(HaveLen(1))
			Expect(assets[0].TagIDs).To(ConsistOf(tagID))
			Expect(assets[0].Tags).To(HaveLen(1))
			Expect(assets[0].Tags[0].ID).To(Equal(tagID))
			Expect(assets[0].Tags[0].Name).To(Equal("Featured"))
		})

		It("returns tags populated on GetByID", func() {
			p := createPiece("Tagged", `[{"type":"paragraph"}]`)

			body, _ := json.Marshal(fiber.Map{
				"title":   "Tagged",
				"content": `[{"type":"paragraph"}]`,
				"tag_ids": []string{tagID},
			})
			req := httptest.NewRequest("PUT", "/api/content-bank/assets/"+p.ID, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(authCookie)
			_, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())

			getReq := httptest.NewRequest("GET", "/api/content-bank/assets/"+p.ID, nil)
			getReq.AddCookie(authCookie)
			getResp, err := app.Test(getReq)
			Expect(err).NotTo(HaveOccurred())
			Expect(getResp.StatusCode).To(Equal(200))

			var got models.Asset
			Expect(json.NewDecoder(getResp.Body).Decode(&got)).To(Succeed())
			Expect(got.Tags).To(HaveLen(1))
			Expect(got.Tags[0].Name).To(Equal("Featured"))
		})

		It("returns empty tags array when no tag_ids set", func() {
			p := createPiece("No Tags", `[{"type":"paragraph"}]`)

			getReq := httptest.NewRequest("GET", "/api/content-bank/assets/"+p.ID, nil)
			getReq.AddCookie(authCookie)
			getResp, err := app.Test(getReq)
			Expect(err).NotTo(HaveOccurred())

			var got models.Asset
			Expect(json.NewDecoder(getResp.Body).Decode(&got)).To(Succeed())
			Expect(got.Tags).NotTo(BeNil())
			Expect(got.Tags).To(BeEmpty())
		})
	})

	// ── Delete ───────────────────────────────────────────────────────────────

	Describe("DELETE /api/content-bank/assets/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("DELETE", "/api/content-bank/assets/someid", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("deletes the asset and returns 204", func() {
				p := createPiece("To Delete", `[]`)

				req := httptest.NewRequest("DELETE", "/api/content-bank/assets/"+p.ID, nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(204))
			})

			It("returns 404 for an unknown id", func() {
				req := httptest.NewRequest("DELETE", "/api/content-bank/assets/nonexistent", nil)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})
})

// ── onSave (embed trigger) ───────────────────────────────────────────────────

var _ = Describe("AssetsHandler onSave embed trigger", Ordered, func() {
	var (
		app          *fiber.App
		db           *bun.DB
		authCookie   *http.Cookie
		onSaveCh     chan string // receives asset IDs whenever onSave fires
		onSaveTenant chan string // receives the tenant id passed to onSave
	)

	BeforeAll(func() {
		db = mustOpenTestDBWithMigrations()
	})

	BeforeEach(func() {
		onSaveCh = make(chan string, 16)
		onSaveTenant = make(chan string, 16)
		// Capture the channels in locals for the onSave closure below. onSave
		// fires in a fire-and-forget goroutine (go h.onSave) that can outlive
		// the spec; reading the package-level vars from there would race with
		// the next spec's BeforeEach reassigning them (a data race under -race).
		saveCh, tenantCh := onSaveCh, onSaveTenant

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
		assetRepo := repository.NewAssetRepository(db, tagRepo, repository.NewAssetFileRepository(db))
		auth := handlers.RequireAuth(sessionRepo, testCookieName)
		handlers.NewUsersHandler(db, userRepo, repository.NewAccountRepository(db), settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, repository.NewAccountRepository(db), sessionRepo, testCookieName, false).Register(app)
		handlers.NewTagsHandler(tagRepo, auth).Register(app)
		handlers.NewAssetsHandler(assetRepo, repository.NewAssetFileRepository(db), nil, nil, nil, auth, func(assetID, _, _, tenantID string) {
			saveCh <- assetID
			tenantCh <- tenantID
		}).Register(app)

		seedTenantUser(db, "Admin", "admin@example.com", "admin-password")

		loginBody, _ := json.Marshal(fiber.Map{"email": "admin@example.com", "password": "admin-password"})
		loginReq := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(loginBody))
		loginReq.Header.Set("Content-Type", "application/json")
		loginResp, err := app.Test(loginReq)
		Expect(err).NotTo(HaveOccurred())
		cookies := loginResp.Cookies()
		Expect(cookies).To(HaveLen(1))
		authCookie = cookies[0]
	})

	AfterEach(func() {
		_, err := db.NewDelete().TableExpr("assets").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("tags").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(context.Background())
		_, err = db.NewDelete().TableExpr("accounts").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})

	// waitForSave returns true if an onSave event arrived within the timeout.
	waitForSave := func() bool {
		select {
		case <-onSaveCh:
			return true
		case <-time.After(200 * time.Millisecond):
			return false
		}
	}

	createAsset := func(title, content string) models.Asset {
		body, _ := json.Marshal(fiber.Map{"title": title, "content": content})
		req := httptest.NewRequest("POST", "/api/content-bank/assets", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		var a models.Asset
		Expect(json.NewDecoder(resp.Body).Decode(&a)).To(Succeed())
		return a
	}

	updateAsset := func(id, title, content string, extra map[string]any) {
		body := fiber.Map{"title": title, "content": content}
		for k, v := range extra {
			body[k] = v
		}
		buf, _ := json.Marshal(body)
		req := httptest.NewRequest("PUT", "/api/content-bank/assets/"+id, bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(200))
	}

	It("fires onSave on create", func() {
		createAsset("Title", "Content body")
		Expect(waitForSave()).To(BeTrue(), "expected onSave to fire on create")
	})

	// CON-97 file-upload bug: the embed runs in a background goroutine with no
	// request context, so the handler must thread the caller's tenant into
	// onSave — otherwise the scheduler's tenant-scoped writes fail closed.
	It("threads the caller's tenant into onSave", func() {
		createAsset("Title", "Content body")
		Expect(waitForSave()).To(BeTrue())
		select {
		case tid := <-onSaveTenant:
			Expect(tid).To(Equal(models.DefaultTenantID))
		case <-time.After(200 * time.Millisecond):
			Fail("expected the caller's tenant to be passed to onSave")
		}
	})

	It("fires onSave when content changes", func() {
		asset := createAsset("Title", "Original content")
		Expect(waitForSave()).To(BeTrue()) // drain create event

		updateAsset(asset.ID, "Title", "Updated content", nil)
		Expect(waitForSave()).To(BeTrue(), "expected onSave to fire on content change")
	})

	It("fires onSave when title changes", func() {
		asset := createAsset("Original Title", "Content")
		Expect(waitForSave()).To(BeTrue()) // drain create event

		updateAsset(asset.ID, "New Title", "Content", nil)
		Expect(waitForSave()).To(BeTrue(), "expected onSave to fire on title change")
	})

	It("does NOT fire onSave when title and content are unchanged", func() {
		asset := createAsset("Title", "Unchanged content")
		Expect(waitForSave()).To(BeTrue()) // drain create event

		updateAsset(asset.ID, "Title", "Unchanged content", nil)
		Expect(waitForSave()).To(BeFalse(), "onSave must not fire when embed inputs are unchanged")
	})

	It("does NOT fire onSave on a tag-only update", func() {
		// Seed a tag.
		tagBody, _ := json.Marshal(fiber.Map{"name": "Q4", "color": "#3498DB"})
		tagReq := httptest.NewRequest("POST", "/api/tags", bytes.NewReader(tagBody))
		tagReq.Header.Set("Content-Type", "application/json")
		tagReq.AddCookie(authCookie)
		tagResp, err := app.Test(tagReq)
		Expect(err).NotTo(HaveOccurred())
		var t map[string]any
		Expect(json.NewDecoder(tagResp.Body).Decode(&t)).To(Succeed())
		tagID := t["id"].(string)

		asset := createAsset("Title", "Body")
		Expect(waitForSave()).To(BeTrue()) // drain create event

		updateAsset(asset.ID, "Title", "Body", map[string]any{"tag_ids": []string{tagID}})
		Expect(waitForSave()).To(BeFalse(), "onSave must not fire for a tag-only change")
	})
})
