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

var _ = Describe("TenantsHandler", Ordered, func() {
	var (
		app *fiber.App
		db  *bun.DB
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
		tenantRepo := repository.NewTenantRepository(db)
		sessionRepo := repository.NewSessionRepository(db)
		auth := handlers.RequireAuth(sessionRepo, testCookieName)
		handlers.NewTenantsHandler(db, tenantRepo, userRepo, testCookieName, false, auth).Register(app)
	})

	AfterEach(func() {
		ctx := context.Background()
		_, _ = db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(ctx)
		// Leave the default tenant (seeded by the migration); drop signup tenants.
		_, _ = db.NewDelete().Model((*models.Tenant)(nil)).
			Where("id <> ?", models.DefaultTenantID).Exec(ctx)
	})

	// signup performs a successful signup and returns the auth cookie + decoded body.
	signup := func(tenantName, name, email, password string) (*http.Cookie, map[string]any) {
		GinkgoHelper()
		body, _ := json.Marshal(fiber.Map{
			"tenant": fiber.Map{"name": tenantName},
			"user":   fiber.Map{"name": name, "email": email, "password": password},
		})
		req := httptest.NewRequest("POST", "/api/tenants", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		var out map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&out)).To(Succeed())
		var cookie *http.Cookie
		for _, ck := range resp.Cookies() {
			if ck.Name == testCookieName {
				cookie = ck
			}
		}
		Expect(cookie).NotTo(BeNil(), "signup must set the session cookie")
		return cookie, out
	}

	tenantID := func(body map[string]any) string {
		t, ok := body["tenant"].(map[string]any)
		Expect(ok).To(BeTrue())
		return t["id"].(string)
	}

	Describe("POST /api/tenants (signup)", func() {
		It("creates a tenant + first user + session and sets a cookie", func() {
			cookie, out := signup("Acme Inc", "Alice", "alice@acme.test", "password-alice")
			Expect(cookie.Value).NotTo(BeEmpty())

			tenant := out["tenant"].(map[string]any)
			Expect(tenant["id"]).NotTo(BeEmpty())
			Expect(tenant["name"]).To(Equal("Acme Inc"))
			Expect(tenant["slug"]).To(Equal("acme-inc"))

			user := out["user"].(map[string]any)
			Expect(user["email"]).To(Equal("alice@acme.test"))
			Expect(user).NotTo(HaveKey("password_hash"))
		})

		It("rejects a duplicate email with 409", func() {
			signup("Acme Inc", "Alice", "dupe@acme.test", "password-alice")

			body, _ := json.Marshal(fiber.Map{
				"tenant": fiber.Map{"name": "Other Co"},
				"user":   fiber.Map{"name": "Bob", "email": "dupe@acme.test", "password": "password-bob"},
			})
			req := httptest.NewRequest("POST", "/api/tenants", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(fiber.StatusConflict))
		})

		It("derives a distinct slug when tenant names collide", func() {
			_, a := signup("Acme Inc", "Alice", "a@acme.test", "password-alice")
			_, b := signup("Acme Inc", "Bob", "b@acme.test", "password-bob")
			Expect(a["tenant"].(map[string]any)["slug"]).To(Equal("acme-inc"))
			Expect(b["tenant"].(map[string]any)["slug"]).To(Equal("acme-inc-2"))
		})

		DescribeTable("returns 400 for invalid payloads",
			func(payload fiber.Map) {
				body, _ := json.Marshal(payload)
				req := httptest.NewRequest("POST", "/api/tenants", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			},
			Entry("missing tenant name", fiber.Map{"user": fiber.Map{"name": "A", "email": "a@x.test", "password": "password-x"}}),
			Entry("missing user email", fiber.Map{"tenant": fiber.Map{"name": "T"}, "user": fiber.Map{"name": "A", "password": "password-x"}}),
			Entry("bad email", fiber.Map{"tenant": fiber.Map{"name": "T"}, "user": fiber.Map{"name": "A", "email": "nope", "password": "password-x"}}),
			Entry("short password", fiber.Map{"tenant": fiber.Map{"name": "T"}, "user": fiber.Map{"name": "A", "email": "a@x.test", "password": "short"}}),
		)
	})

	Describe("GET /api/tenants/current", func() {
		It("returns 401 without a session", func() {
			req := httptest.NewRequest("GET", "/api/tenants/current", nil)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(401))
		})

		It("returns the caller's tenant", func() {
			cookie, out := signup("Acme Inc", "Alice", "alice@acme.test", "password-alice")
			req := httptest.NewRequest("GET", "/api/tenants/current", nil)
			req.AddCookie(cookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var got map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
			Expect(got["id"]).To(Equal(tenantID(out)))
		})
	})

	Describe("GET /api/tenants/:id (isolation)", func() {
		It("returns the caller's own tenant", func() {
			cookie, out := signup("Acme Inc", "Alice", "alice@acme.test", "password-alice")
			id := tenantID(out)
			req := httptest.NewRequest("GET", "/api/tenants/"+id, nil)
			req.AddCookie(cookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
		})

		It("returns 404 for another tenant's id (no existence leak)", func() {
			cookieA, _ := signup("Acme Inc", "Alice", "alice@acme.test", "password-alice")
			_, outB := signup("Beta Co", "Bob", "bob@beta.test", "password-bob")
			idB := tenantID(outB)

			req := httptest.NewRequest("GET", "/api/tenants/"+idB, nil)
			req.AddCookie(cookieA)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(404))
		})
	})

	Describe("PUT /api/tenants/:id", func() {
		It("updates the caller's own tenant name", func() {
			cookie, out := signup("Acme Inc", "Alice", "alice@acme.test", "password-alice")
			id := tenantID(out)

			body, _ := json.Marshal(fiber.Map{"name": "Acme Renamed"})
			req := httptest.NewRequest("PUT", "/api/tenants/"+id, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(cookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var got map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
			Expect(got["name"]).To(Equal("Acme Renamed"))
			// Slug is stable across renames.
			Expect(got["slug"]).To(Equal("acme-inc"))
		})

		It("returns 404 when updating another tenant", func() {
			cookieA, _ := signup("Acme Inc", "Alice", "alice@acme.test", "password-alice")
			_, outB := signup("Beta Co", "Bob", "bob@beta.test", "password-bob")
			idB := tenantID(outB)

			body, _ := json.Marshal(fiber.Map{"name": "Hacked"})
			req := httptest.NewRequest("PUT", "/api/tenants/"+idB, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(cookieA)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(404))
		})

		It("returns 400 for a blank name", func() {
			cookie, out := signup("Acme Inc", "Alice", "alice@acme.test", "password-alice")
			id := tenantID(out)

			body, _ := json.Marshal(fiber.Map{"name": ""})
			req := httptest.NewRequest("PUT", "/api/tenants/"+id, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(cookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(400))
		})
	})
})
