package handlers_test

import (
	"encoding/json"
	"io"
	"net/http/httptest"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/handlers"
	"github.com/ogen-app/ogen/src/pgtest"
)

var _ = Describe("HealthHandler", func() {
	var (
		app *fiber.App
		db  = mustOpenTestDB()
	)

	BeforeEach(func() {
		app = fiber.New()
		handlers.NewHealthHandler(db, nil).Register(app)
	})

	Describe("GET /api/health", func() {
		Context("when the database is reachable", func() {
			It("returns 200 with status ok", func() {
				req := httptest.NewRequest("GET", "/api/health", nil)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				body, _ := io.ReadAll(resp.Body)
				var payload map[string]string
				Expect(json.Unmarshal(body, &payload)).To(Succeed())
				Expect(payload["status"]).To(Equal("ok"))
			})
		})
	})
})

func mustOpenTestDB() *bun.DB {
	return pgtest.MustDB()
}
