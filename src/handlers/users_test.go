package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/database"
	"github.com/content-control-center/app/src/handlers"
	"github.com/content-control-center/app/src/models"
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
		handlers.NewUsersHandler(db).Register(app)
	})

	AfterEach(func() {
		_, err := db.NewDelete().TableExpr("users").Where("1 = 1").Exec(context.Background())
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

	// ── List ─────────────────────────────────────────────────────────────────

	Describe("GET /api/users", func() {
		Context("when no users exist", func() {
			It("returns an empty array", func() {
				req := httptest.NewRequest("GET", "/api/users", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var users []models.User
				Expect(json.NewDecoder(resp.Body).Decode(&users)).To(Succeed())
				Expect(users).To(BeEmpty())
			})
		})

		Context("when users exist", func() {
			BeforeEach(func() {
				createUser("Alice", "alice@example.com", "password-alice")
				createUser("Bob", "bob@example.com", "password-bob")
			})

			It("returns all users", func() {
				req := httptest.NewRequest("GET", "/api/users", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var users []models.User
				Expect(json.NewDecoder(resp.Body).Decode(&users)).To(Succeed())
				Expect(users).To(HaveLen(2))
			})

			It("does not expose password_hash in the response", func() {
				req := httptest.NewRequest("GET", "/api/users", nil)
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
			It("creates a user and returns 201 with a Sqid", func() {
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
		Context("when the user exists", func() {
			It("returns the user without password hash", func() {
				created := createUser("Dave", "dave@example.com", "password-dave")

				req := httptest.NewRequest("GET", fmt.Sprintf("/api/users/%s", created.ID), nil)
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

		Context("when the user does not exist", func() {
			It("returns 404", func() {
				req := httptest.NewRequest("GET", "/api/users/doesnotexist", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})

	// ── Update ───────────────────────────────────────────────────────────────

	Describe("PUT /api/users/:id", func() {
		Context("when the user exists", func() {
			It("updates name and email without changing the password", func() {
				created := createUser("Eve", "eve@example.com", "password-eve")

				body, _ := json.Marshal(fiber.Map{"name": "Eve Updated", "email": "eve2@example.com"})
				req := httptest.NewRequest("PUT", fmt.Sprintf("/api/users/%s", created.ID), bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
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

				body, _ := json.Marshal(fiber.Map{"name": "Eve2", "email": "eve2@example.com", "password": "new-password"})
				req := httptest.NewRequest("PUT", fmt.Sprintf("/api/users/%s", created.ID), bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
			})
		})

		Context("when the user does not exist", func() {
			It("returns 404", func() {
				body, _ := json.Marshal(fiber.Map{"name": "X", "email": "x@example.com"})
				req := httptest.NewRequest("PUT", "/api/users/doesnotexist", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})

		Context("with invalid payload", func() {
			DescribeTable("returns 400",
				func(payload fiber.Map) {
					created := createUser("Frank", "frank@example.com", "password-frank")
					body, _ := json.Marshal(payload)
					req := httptest.NewRequest("PUT", fmt.Sprintf("/api/users/%s", created.ID), bytes.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
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
		Context("when the user exists", func() {
			It("deletes the user and returns 204", func() {
				created := createUser("Grace", "grace@example.com", "password-grace")

				req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/users/%s", created.ID), nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(204))

				body, _ := io.ReadAll(resp.Body)
				Expect(body).To(BeEmpty())
			})

			It("returns 404 on subsequent get", func() {
				created := createUser("Henry", "henry@example.com", "password-henry")

				app.Test(httptest.NewRequest("DELETE", fmt.Sprintf("/api/users/%s", created.ID), nil)) //nolint:errcheck

				req := httptest.NewRequest("GET", fmt.Sprintf("/api/users/%s", created.ID), nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})

		Context("when the user does not exist", func() {
			It("returns 404", func() {
				req := httptest.NewRequest("DELETE", "/api/users/doesnotexist", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})
})

func mustOpenTestDBWithMigrations() *bun.DB {
	db, err := database.New("file::memory:?cache=shared&_pragma=foreign_keys(on)", false)
	if err != nil {
		panic(err)
	}
	if err := database.Migrate(context.Background(), db); err != nil {
		panic(err)
	}
	return db
}
