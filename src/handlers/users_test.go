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

	createUser := func(name, email string) *models.User {
		body, _ := json.Marshal(fiber.Map{"name": name, "email": email})
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
				createUser("Alice", "alice@example.com")
				createUser("Bob", "bob@example.com")
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
		})
	})

	// ── Create ───────────────────────────────────────────────────────────────

	Describe("POST /api/users", func() {
		Context("with valid payload", func() {
			It("creates a user and returns 201 with a Sqid", func() {
				u := createUser("Carol", "carol@example.com")
				Expect(u.ID).NotTo(BeEmpty())
				Expect(u.Name).To(Equal("Carol"))
				Expect(u.Email).To(Equal("carol@example.com"))
			})
		})

		Context("with missing fields", func() {
			DescribeTable("returns 400",
				func(payload fiber.Map) {
					body, _ := json.Marshal(payload)
					req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					resp, err := app.Test(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(400))
				},
				Entry("missing name", fiber.Map{"email": "x@example.com"}),
				Entry("missing email", fiber.Map{"name": "X"}),
				Entry("empty body", fiber.Map{}),
			)
		})
	})

	// ── Get ──────────────────────────────────────────────────────────────────

	Describe("GET /api/users/:id", func() {
		Context("when the user exists", func() {
			It("returns the user", func() {
				created := createUser("Dave", "dave@example.com")

				req := httptest.NewRequest("GET", fmt.Sprintf("/api/users/%s", created.ID), nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var u models.User
				Expect(json.NewDecoder(resp.Body).Decode(&u)).To(Succeed())
				Expect(u.ID).To(Equal(created.ID))
				Expect(u.Email).To(Equal("dave@example.com"))
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
			It("updates and returns the user", func() {
				created := createUser("Eve", "eve@example.com")

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

		Context("with missing fields", func() {
			It("returns 400", func() {
				created := createUser("Frank", "frank@example.com")

				body, _ := json.Marshal(fiber.Map{"name": "Frank"})
				req := httptest.NewRequest("PUT", fmt.Sprintf("/api/users/%s", created.ID), bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})
		})
	})

	// ── Delete ───────────────────────────────────────────────────────────────

	Describe("DELETE /api/users/:id", func() {
		Context("when the user exists", func() {
			It("deletes the user and returns 204", func() {
				created := createUser("Grace", "grace@example.com")

				req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/users/%s", created.ID), nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(204))

				body, _ := io.ReadAll(resp.Body)
				Expect(body).To(BeEmpty())
			})

			It("returns 404 on subsequent get", func() {
				created := createUser("Henry", "henry@example.com")

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
