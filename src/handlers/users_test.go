package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/database"
	"github.com/content-control-center/app/src/handlers"
	"github.com/content-control-center/app/src/models"
	"github.com/content-control-center/app/src/repository"
)

var _ = Describe("UsersHandler", Ordered, func() {
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
		sessionRepo := repository.NewSessionRepository(db)
		settingRepo := repository.NewSettingRepository(db)
		auth := handlers.RequireAuth(sessionRepo, testCookieName)
		handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(app)
		handlers.NewSettingsHandler(settingRepo, auth).Register(app)
	})

	AfterEach(func() {
		_, err := db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewUpdate().TableExpr("settings").Set("value = ?", "false").
			Where("key = ?", "setup_complete").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})

	// ── helpers ──────────────────────────────────────────────────────────────

	createUser := func(name, email, password string) *models.User {
		body, _ := json.Marshal(fiber.Map{"name": name, "email": email, "password": password})
		req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		var u models.User
		Expect(json.NewDecoder(resp.Body).Decode(&u)).To(Succeed())
		return &u
	}

	loginAs := func(email, password string) *http.Cookie {
		body, _ := json.Marshal(fiber.Map{"email": email, "password": password})
		req := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		cookies := resp.Cookies()
		Expect(cookies).To(HaveLen(1))
		return cookies[0]
	}

	// ── CurrentUser ──────────────────────────────────────────────────────────

	Describe("GET /api/current_user", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/current_user", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns the authenticated user", func() {
				u := createUser("Alice", "alice@example.com", "password-alice")
				cookie := loginAs("alice@example.com", "password-alice")

				req := httptest.NewRequest("GET", "/api/current_user", nil)
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var got models.User
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got.ID).To(Equal(u.ID))
				Expect(got.Name).To(Equal("Alice"))
				Expect(got.Email).To(Equal("alice@example.com"))
			})

			It("does not expose password_hash in the response", func() {
				createUser("Alice", "alice@example.com", "password-alice")
				cookie := loginAs("alice@example.com", "password-alice")

				req := httptest.NewRequest("GET", "/api/current_user", nil)
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())

				var raw map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&raw)).To(Succeed())
				Expect(raw).NotTo(HaveKey("password_hash"))
				Expect(raw).NotTo(HaveKey("PasswordHash"))
			})
		})
	})

	// ── List ─────────────────────────────────────────────────────────────────

	Describe("GET /api/users", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/users", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns all users", func() {
				createUser("Alice", "alice@example.com", "password-alice")
				createUser("Bob", "bob@example.com", "password-bob")
				cookie := loginAs("alice@example.com", "password-alice")

				req := httptest.NewRequest("GET", "/api/users", nil)
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var users []models.User
				Expect(json.NewDecoder(resp.Body).Decode(&users)).To(Succeed())
				Expect(users).To(HaveLen(2))
			})

			It("does not expose password_hash in the response", func() {
				createUser("Alice", "alice@example.com", "password-alice")
				cookie := loginAs("alice@example.com", "password-alice")

				req := httptest.NewRequest("GET", "/api/users", nil)
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())

				var raw []map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&raw)).To(Succeed())
				for _, u := range raw {
					Expect(u).NotTo(HaveKey("password_hash"))
					Expect(u).NotTo(HaveKey("PasswordHash"))
				}
			})
		})
	})

	// ── Create ───────────────────────────────────────────────────────────────

	Describe("POST /api/users", func() {
		Context("with valid payload", func() {
			It("creates a user and returns 201 with a Sqid — no auth required", func() {
				u := createUser("Carol", "carol@example.com", "s3cur3P@ss")
				Expect(u.ID).NotTo(BeEmpty())
				Expect(u.Name).To(Equal("Carol"))
				Expect(u.Email).To(Equal("carol@example.com"))
			})

			It("does not expose the password hash in the response", func() {
				body, _ := json.Marshal(fiber.Map{"name": "X", "email": "x@example.com", "password": "s3cur3P@ss"})
				req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())

				var raw map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&raw)).To(Succeed())
				Expect(raw).NotTo(HaveKey("password_hash"))
				Expect(raw).NotTo(HaveKey("PasswordHash"))
				Expect(raw).NotTo(HaveKey("password"))
			})
		})

		Context("with invalid payload", func() {
			DescribeTable("returns 400",
				func(payload fiber.Map) {
					body, _ := json.Marshal(payload)
					req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					resp, err := app.Test(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(400))
				},
				Entry("missing name", fiber.Map{"email": "x@example.com", "password": "s3cur3P@ss"}),
				Entry("missing email", fiber.Map{"name": "X", "password": "s3cur3P@ss"}),
				Entry("missing password", fiber.Map{"name": "X", "email": "x@example.com"}),
				Entry("empty body", fiber.Map{}),
				Entry("invalid email format", fiber.Map{"name": "X", "email": "not-an-email", "password": "s3cur3P@ss"}),
				Entry("email missing domain", fiber.Map{"name": "X", "email": "user@", "password": "s3cur3P@ss"}),
				Entry("email missing @", fiber.Map{"name": "X", "email": "userexample.com", "password": "s3cur3P@ss"}),
				Entry("password too short", fiber.Map{"name": "X", "email": "x@example.com", "password": "short"}),
			)

			It("returns a descriptive error message for invalid email", func() {
				body, _ := json.Marshal(fiber.Map{"name": "X", "email": "bad-email", "password": "s3cur3P@ss"})
				req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))

				var payload map[string]string
				Expect(json.NewDecoder(resp.Body).Decode(&payload)).To(Succeed())
				Expect(payload["error"]).To(ContainSubstring("email"))
			})

			It("returns a descriptive error message for short password", func() {
				body, _ := json.Marshal(fiber.Map{"name": "X", "email": "x@example.com", "password": "short"})
				req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))

				var payload map[string]string
				Expect(json.NewDecoder(resp.Body).Decode(&payload)).To(Succeed())
				Expect(payload["error"]).To(ContainSubstring("password"))
			})
		})
	})

	// ── Get ──────────────────────────────────────────────────────────────────

	Describe("GET /api/users/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("GET", "/api/users/someid", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated and the user exists", func() {
			It("returns the user without password hash", func() {
				created := createUser("Dave", "dave@example.com", "password-dave")
				cookie := loginAs("dave@example.com", "password-dave")

				req := httptest.NewRequest("GET", fmt.Sprintf("/api/users/%s", created.ID), nil)
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var raw map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&raw)).To(Succeed())
				Expect(raw["id"]).To(Equal(created.ID))
				Expect(raw["email"]).To(Equal("dave@example.com"))
				Expect(raw).NotTo(HaveKey("password_hash"))
			})
		})

		Context("when authenticated and the user does not exist", func() {
			It("returns 404", func() {
				createUser("Dave", "dave@example.com", "password-dave")
				cookie := loginAs("dave@example.com", "password-dave")

				req := httptest.NewRequest("GET", "/api/users/doesnotexist", nil)
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})

	// ── Update ───────────────────────────────────────────────────────────────

	Describe("PUT /api/users/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{"name": "X", "email": "x@example.com"})
				req := httptest.NewRequest("PUT", "/api/users/someid", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated and the user exists", func() {
			It("updates name and email without changing the password", func() {
				created := createUser("Eve", "eve@example.com", "password-eve")
				cookie := loginAs("eve@example.com", "password-eve")

				body, _ := json.Marshal(fiber.Map{"name": "Eve Updated", "email": "eve2@example.com"})
				req := httptest.NewRequest("PUT", fmt.Sprintf("/api/users/%s", created.ID), bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var u models.User
				Expect(json.NewDecoder(resp.Body).Decode(&u)).To(Succeed())
				Expect(u.Name).To(Equal("Eve Updated"))
				Expect(u.Email).To(Equal("eve2@example.com"))
			})

			It("updates the password when provided", func() {
				created := createUser("Eve2", "eve2@example.com", "old-password")
				cookie := loginAs("eve2@example.com", "old-password")

				body, _ := json.Marshal(fiber.Map{"name": "Eve2", "email": "eve2@example.com", "password": "new-password"})
				req := httptest.NewRequest("PUT", fmt.Sprintf("/api/users/%s", created.ID), bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
			})
		})

		Context("when authenticated as a different user", func() {
			It("returns 403 and does not modify the target user", func() {
				target := createUser("Target", "target@example.com", "password-target")
				createUser("Other", "other@example.com", "password-other")
				cookie := loginAs("other@example.com", "password-other")

				body, _ := json.Marshal(fiber.Map{"name": "Hacked", "email": "hacked@example.com"})
				req := httptest.NewRequest("PUT", fmt.Sprintf("/api/users/%s", target.ID), bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))

				// The target user is unchanged and the original password still works.
				Expect(loginAs("target@example.com", "password-target")).NotTo(BeNil())
			})
		})

		Context("when authenticated as self but the route id does not exist", func() {
			It("returns 403", func() {
				createUser("Frank", "frank@example.com", "password-frank")
				cookie := loginAs("frank@example.com", "password-frank")

				body, _ := json.Marshal(fiber.Map{"name": "X", "email": "x@example.com"})
				req := httptest.NewRequest("PUT", "/api/users/doesnotexist", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))
			})
		})

		Context("when authenticated with invalid payload", func() {
			DescribeTable("returns 400",
				func(payload fiber.Map) {
					created := createUser("Frank", "frank@example.com", "password-frank")
					cookie := loginAs("frank@example.com", "password-frank")

					body, _ := json.Marshal(payload)
					req := httptest.NewRequest("PUT", fmt.Sprintf("/api/users/%s", created.ID), bytes.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					req.AddCookie(cookie)
					resp, err := app.Test(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(400))
				},
				Entry("missing email", fiber.Map{"name": "Frank"}),
				Entry("missing name", fiber.Map{"email": "frank@example.com"}),
				Entry("invalid email format", fiber.Map{"name": "Frank", "email": "not-an-email"}),
				Entry("password too short", fiber.Map{"name": "Frank", "email": "frank@example.com", "password": "short"}),
			)
		})
	})

	// ── Delete ───────────────────────────────────────────────────────────────

	Describe("DELETE /api/users/:id", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("DELETE", "/api/users/someid", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated and the user exists", func() {
			It("deletes the user and returns 204", func() {
				created := createUser("Grace", "grace@example.com", "password-grace")
				cookie := loginAs("grace@example.com", "password-grace")

				req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/users/%s", created.ID), nil)
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(204))

				body, _ := io.ReadAll(resp.Body)
				Expect(body).To(BeEmpty())
			})

			It("makes the user unreachable on subsequent get", func() {
				henry := createUser("Henry", "henry@example.com", "password-henry")
				cookie := loginAs("henry@example.com", "password-henry")

				delReq := httptest.NewRequest("DELETE", fmt.Sprintf("/api/users/%s", henry.ID), nil)
				delReq.AddCookie(cookie)
				delResp, delErr := app.Test(delReq)
				Expect(delErr).NotTo(HaveOccurred())
				Expect(delResp.StatusCode).To(Equal(204))

				// Session was tied to the deleted user, so the follow-up Get is now
				// unauthenticated and the auth middleware returns 401.
				getReq := httptest.NewRequest("GET", fmt.Sprintf("/api/users/%s", henry.ID), nil)
				getReq.AddCookie(cookie)
				resp, err := app.Test(getReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated as a different user", func() {
			It("returns 403 and does not delete the target user", func() {
				target := createUser("Target", "target@example.com", "password-target")
				createUser("Other", "other@example.com", "password-other")
				cookie := loginAs("other@example.com", "password-other")

				req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/users/%s", target.ID), nil)
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))

				// Target still exists and can still log in.
				Expect(loginAs("target@example.com", "password-target")).NotTo(BeNil())
			})
		})

		Context("when authenticated as self but the route id does not exist", func() {
			It("returns 403", func() {
				createUser("Iris", "iris@example.com", "password-iris")
				cookie := loginAs("iris@example.com", "password-iris")

				req := httptest.NewRequest("DELETE", "/api/users/doesnotexist", nil)
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))
			})
		})
	})

	// ── Conditional auth on Create ────────────────────────────────────────────

	Describe("POST /api/users conditional auth", func() {
		setSetupComplete := func(value string) {
			body, _ := json.Marshal(fiber.Map{"value": value})
			req := httptest.NewRequest("PUT", "/api/settings/setup_complete", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			// Need a valid session to update settings — create a temp user and log in
			createUser("TempAdmin", "tempadmin@example.com", "temp-password")
			cookie := loginAs("tempadmin@example.com", "temp-password")
			req.AddCookie(cookie)

			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))
		}

		Context("when setup_complete is false", func() {
			It("allows user creation without authentication", func() {
				body, _ := json.Marshal(fiber.Map{
					"name": "New User", "email": "new@example.com", "password": "new-password",
				})
				req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
			})
		})

		Context("when setup_complete is true", func() {
			BeforeEach(func() {
				setSetupComplete("true")
			})

			It("rejects unauthenticated user creation with 401", func() {
				body, _ := json.Marshal(fiber.Map{
					"name": "Another User", "email": "another@example.com", "password": "another-password",
				})
				req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})

			It("allows user creation with a valid session", func() {
				cookie := loginAs("tempadmin@example.com", "temp-password")

				body, _ := json.Marshal(fiber.Map{
					"name": "Another User", "email": "another@example.com", "password": "another-password",
				})
				req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.AddCookie(cookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
			})
		})
	})
})

// testDBSeq guarantees every call to mustOpenTestDBWithMigrations gets a
// distinct in-memory database — see the function comment for why.
var testDBSeq atomic.Uint64

// mustOpenTestDBWithMigrations returns a fresh, isolated in-memory
// SQLite DB with all migrations applied (CON-70).
//
// The previous shape used a fixed DSN (`file::memory:?cache=shared`)
// shared across every Describe in the package, which under ginkgo's
// `-procs=N` + `-race` produced layered flakes:
//
//   1. Cross-Describe state leak. Within one ginkgo worker, every
//      Describe's BeforeAll opened a connection to the same shared
//      cache; each new Describe inherited rows the previous one
//      forgot to delete. Per-call uniqueness fixes this — every
//      BeforeAll gets its own DB name (worker prefix + counter).
//   2. Connection-pool starvation. database.New caps MaxOpenConns at
//      1 (correct for production SQLite with WAL — minimises lock
//      thrashing). Under `-race` + `-procs=2`, the single connection
//      becomes the bottleneck: a Fiber handler request waits on the
//      pool while a concurrent BeforeEach query holds it, easily
//      blowing fiber's 1000ms default `app.Test(req)` timeout. For an
//      in-memory test DB the production trade-off doesn't apply, so
//      we explicitly bump MaxOpenConns to leave headroom for parallel
//      handler requests.
//
// cache=shared is preserved so multiple connections from the bumped
// pool still see the same data. GinkgoParallelProcess() keeps
// per-OS-process workers from colliding even though in-memory SQLite
// is process-local — belt and braces, makes log lines easier to read
// when a worker emits its first migration line.
func mustOpenTestDBWithMigrations() *bun.DB {
	n := testDBSeq.Add(1)
	dsn := fmt.Sprintf(
		"file:test_p%d_%d?mode=memory&cache=shared&_pragma=foreign_keys(on)",
		GinkgoParallelProcess(), n,
	)
	db, err := database.New(dsn, false)
	if err != nil {
		panic(err)
	}
	// Override database.New's production SQLite single-conn cap. See
	// the function-level comment for the rationale.
	db.DB.SetMaxOpenConns(10)
	db.DB.SetMaxIdleConns(10)
	if err := database.Migrate(context.Background(), db); err != nil {
		panic(err)
	}
	return db
}
