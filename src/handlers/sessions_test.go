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

var _ = Describe("SessionsHandler", Ordered, func() {
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

	seedUser := func(name, email, password string) {
		body, _ := json.Marshal(fiber.Map{"name": name, "email": email, "password": password})
		req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
	}

	doLogin := func(email, password string) (*http.Response, models.Session) {
		body, _ := json.Marshal(fiber.Map{"email": email, "password": password})
		req := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		var session models.Session
		if resp.StatusCode == fiber.StatusCreated {
			Expect(json.NewDecoder(resp.Body).Decode(&session)).To(Succeed())
		}
		return resp, session
	}

	// ── Create (login) ───────────────────────────────────────────────────────

	Describe("POST /api/sessions", func() {
		Context("with valid credentials", func() {
			It("returns 201 with session body and sets an HttpOnly cookie", func() {
				seedUser("Alice", "alice@example.com", "password-alice")

				resp, session := doLogin("alice@example.com", "password-alice")
				Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
				Expect(session.ID).NotTo(BeEmpty())
				Expect(session.UserID).NotTo(BeEmpty())
				Expect(session.ExpiresAt.IsZero()).To(BeFalse())

				cookies := resp.Cookies()
				Expect(cookies).To(HaveLen(1))
				Expect(cookies[0].Name).To(Equal(testCookieName))
				Expect(cookies[0].Value).To(Equal(session.ID))
				Expect(cookies[0].HttpOnly).To(BeTrue())
				// This suite wires the handler with secureCookie=false (dev mode),
				// so the Secure flag must NOT be set on the cookie.
				Expect(cookies[0].Secure).To(BeFalse())
			})

			It("sets the Secure flag when the handler is configured for production", func() {
				secureApp := fiber.New(fiber.Config{
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
				handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(secureApp)
				handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, true).Register(secureApp)

				// Seed a user via the production-wired app so we get setup_complete=false handling.
				body, _ := json.Marshal(fiber.Map{"name": "Sec", "email": "sec@example.com", "password": "password-sec"})
				createReq := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
				createReq.Header.Set("Content-Type", "application/json")
				createResp, err := secureApp.Test(createReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(createResp.StatusCode).To(Equal(fiber.StatusCreated))

				loginBody, _ := json.Marshal(fiber.Map{"email": "sec@example.com", "password": "password-sec"})
				loginReq := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(loginBody))
				loginReq.Header.Set("Content-Type", "application/json")
				loginResp, err := secureApp.Test(loginReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(loginResp.StatusCode).To(Equal(fiber.StatusCreated))

				cookies := loginResp.Cookies()
				Expect(cookies).To(HaveLen(1))
				Expect(cookies[0].Secure).To(BeTrue())
				Expect(cookies[0].HttpOnly).To(BeTrue())
			})
		})

		Context("with wrong password", func() {
			It("returns 401", func() {
				seedUser("Bob", "bob@example.com", "correct-password")

				body, _ := json.Marshal(fiber.Map{"email": "bob@example.com", "password": "wrong-password"})
				req := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusUnauthorized))
			})
		})

		Context("with unknown email", func() {
			It("returns 401", func() {
				body, _ := json.Marshal(fiber.Map{"email": "nobody@example.com", "password": "whatever"})
				req := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusUnauthorized))
			})
		})

		Context("with invalid payload", func() {
			DescribeTable("returns 400",
				func(payload fiber.Map) {
					body, _ := json.Marshal(payload)
					req := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					resp, err := app.Test(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(fiber.StatusBadRequest))
				},
				Entry("missing email", fiber.Map{"password": "s3cur3P@ss"}),
				Entry("missing password", fiber.Map{"email": "x@example.com"}),
				Entry("invalid email format", fiber.Map{"email": "not-an-email", "password": "s3cur3P@ss"}),
				Entry("empty body", fiber.Map{}),
			)
		})
	})

	// ── Delete (logout) ──────────────────────────────────────────────────────

	Describe("DELETE /api/sessions", func() {
		Context("with a valid session cookie", func() {
			It("returns 204 and clears the cookie", func() {
				seedUser("Carol", "carol@example.com", "password-carol")
				_, session := doLogin("carol@example.com", "password-carol")

				delReq := httptest.NewRequest("DELETE", "/api/sessions", nil)
				delReq.AddCookie(&http.Cookie{Name: testCookieName, Value: session.ID})
				delResp, err := app.Test(delReq)
				Expect(err).NotTo(HaveOccurred())
				Expect(delResp.StatusCode).To(Equal(fiber.StatusNoContent))

				cookies := delResp.Cookies()
				Expect(cookies).To(HaveLen(1))
				Expect(cookies[0].Name).To(Equal(testCookieName))
				Expect(cookies[0].Value).To(BeEmpty())
			})

			It("returns 401 on a second logout with the same token", func() {
				seedUser("Dave", "dave@example.com", "password-dave")
				_, session := doLogin("dave@example.com", "password-dave")

				delReq1 := httptest.NewRequest("DELETE", "/api/sessions", nil)
				delReq1.AddCookie(&http.Cookie{Name: testCookieName, Value: session.ID})
				app.Test(delReq1) //nolint:errcheck

				delReq2 := httptest.NewRequest("DELETE", "/api/sessions", nil)
				delReq2.AddCookie(&http.Cookie{Name: testCookieName, Value: session.ID})
				delResp2, err := app.Test(delReq2)
				Expect(err).NotTo(HaveOccurred())
				Expect(delResp2.StatusCode).To(Equal(fiber.StatusUnauthorized))
			})
		})

		Context("without a cookie", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("DELETE", "/api/sessions", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusUnauthorized))
			})
		})

		Context("with an invalid token in the cookie", func() {
			It("returns 401", func() {
				req := httptest.NewRequest("DELETE", "/api/sessions", nil)
				req.AddCookie(&http.Cookie{Name: testCookieName, Value: "bogus-token"})
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusUnauthorized))
			})
		})
	})
})
