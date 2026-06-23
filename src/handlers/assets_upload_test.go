package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/handlers"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

var _ = Describe("AssetsHandler upload", Ordered, func() {
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
			BodyLimit: 100 << 20,
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
		handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(app)
		handlers.NewAssetsHandler(assetRepo, repository.NewAssetFileRepository(db), nil, auth, nil, nil).Register(app)

		seedTenantUser(db, "Admin", "up@example.com", "pw-password")

		loginBody, _ := json.Marshal(fiber.Map{"email": "up@example.com", "password": "pw-password"})
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
		_, err = db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
		_, err = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(context.Background())
		Expect(err).NotTo(HaveOccurred())
	})

	buildMultipart := func(files []struct{ Name, Body string }) (*bytes.Buffer, string) {
		buf := &bytes.Buffer{}
		w := multipart.NewWriter(buf)
		for _, f := range files {
			fw, err := w.CreateFormFile("files", f.Name)
			Expect(err).NotTo(HaveOccurred())
			_, err = io.Copy(fw, strings.NewReader(f.Body))
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(w.Close()).To(Succeed())
		return buf, w.FormDataContentType()
	}

	post := func(body *bytes.Buffer, ct string) *http.Response {
		req := httptest.NewRequest("POST", "/api/content-bank/assets/upload", body)
		req.Header.Set("Content-Type", ct)
		req.AddCookie(authCookie)
		resp, err := app.Test(req, -1)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	decode := func(resp *http.Response) []map[string]any {
		var out struct {
			Results []map[string]any `json:"results"`
		}
		Expect(json.NewDecoder(resp.Body).Decode(&out)).To(Succeed())
		return out.Results
	}

	It("creates assets from multiple .md files", func() {
		body, ct := buildMultipart([]struct{ Name, Body string }{
			{"first.md", "# Hello\n\nThis is the first asset.\n"},
			{"second.md", "- item 1\n- item 2\n"},
		})
		resp := post(body, ct)
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		results := decode(resp)
		Expect(results).To(HaveLen(2))
		for _, r := range results {
			Expect(r["status"]).To(Equal("created"))
			Expect(r["asset_id"]).NotTo(BeEmpty())
			asset := r["asset"].(map[string]any)
			Expect(asset["type"]).To(Equal("MD"))
			Expect(asset["status"]).To(Equal(models.AssetStatusPending))
		}
		// Titles default to filename without extension.
		Expect(results[0]["filename"]).To(Equal("first.md"))
		Expect(results[0]["asset"].(map[string]any)["title"]).To(Equal("first"))
		Expect(results[1]["asset"].(map[string]any)["title"]).To(Equal("second"))
	})

	It("rejects non-.md files per file but still processes good ones", func() {
		body, ct := buildMultipart([]struct{ Name, Body string }{
			{"bad.txt", "not markdown"},
			{"good.md", "hello"},
		})
		resp := post(body, ct)
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		results := decode(resp)
		Expect(results).To(HaveLen(2))
		Expect(results[0]["status"]).To(Equal("failed"))
		Expect(results[0]["error"]).To(ContainSubstring(".md"))
		Expect(results[1]["status"]).To(Equal("created"))
	})

	It("rejects files over the size limit", func() {
		big := strings.Repeat("x", (10<<20)+1)
		body, ct := buildMultipart([]struct{ Name, Body string }{{"big.md", big}})
		resp := post(body, ct)
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		results := decode(resp)
		Expect(results).To(HaveLen(1))
		Expect(results[0]["status"]).To(Equal("failed"))
		Expect(results[0]["error"]).To(ContainSubstring("exceeds"))
	})

	It("creates an empty asset for an empty .md file", func() {
		body, ct := buildMultipart([]struct{ Name, Body string }{{"empty.md", ""}})
		resp := post(body, ct)
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		results := decode(resp)
		Expect(results).To(HaveLen(1))
		Expect(results[0]["status"]).To(Equal("created"))
		asset := results[0]["asset"].(map[string]any)
		Expect(asset["content"]).To(Equal(""))
		Expect(asset["type"]).To(Equal("MD"))
	})

	It("returns 400 with no files", func() {
		body, ct := buildMultipart(nil)
		resp := post(body, ct)
		Expect(resp.StatusCode).To(Equal(fiber.StatusBadRequest))
	})

	It("creates a PDF asset with type=PDF, status=pending (async processing deferred)", func() {
		// Minimal valid PDF magic header followed by plausible body. The handler
		// only checks the `%PDF` prefix and size; the async flow is not fired
		// because onPDF is nil in this suite.
		body, ct := buildMultipart([]struct{ Name, Body string }{
			{"report.pdf", "%PDF-1.4\n%...dummy body..."},
		})
		resp := post(body, ct)
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		results := decode(resp)
		Expect(results).To(HaveLen(1))
		Expect(results[0]["status"]).To(Equal("created"))
		asset := results[0]["asset"].(map[string]any)
		Expect(asset["type"]).To(Equal(models.AssetTypePDF))
		Expect(asset["status"]).To(Equal(models.AssetStatusPending))
		Expect(asset["title"]).To(Equal("report"))
	})

	It("rejects .pdf files failing the magic-byte sniff", func() {
		body, ct := buildMultipart([]struct{ Name, Body string }{
			{"fake.pdf", "this is not a PDF"},
		})
		resp := post(body, ct)
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		results := decode(resp)
		Expect(results).To(HaveLen(1))
		Expect(results[0]["status"]).To(Equal("failed"))
		Expect(results[0]["error"]).To(ContainSubstring("PDF"))
	})

	It("rejects .pdf files over 50 MB", func() {
		big := "%PDF-1.4\n" + strings.Repeat("x", (50<<20)+1)
		body, ct := buildMultipart([]struct{ Name, Body string }{{"huge.pdf", big}})
		resp := post(body, ct)
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		results := decode(resp)
		Expect(results).To(HaveLen(1))
		Expect(results[0]["status"]).To(Equal("failed"))
		Expect(results[0]["error"]).To(ContainSubstring("exceeds"))
	})

	It("rejects unknown extensions with a clear message", func() {
		body, ct := buildMultipart([]struct{ Name, Body string }{{"notes.txt", "plain text"}})
		resp := post(body, ct)
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		results := decode(resp)
		Expect(results).To(HaveLen(1))
		Expect(results[0]["status"]).To(Equal("failed"))
		Expect(results[0]["error"]).To(ContainSubstring(".pdf"))
	})

	It("requires authentication", func() {
		body, ct := buildMultipart([]struct{ Name, Body string }{{"x.md", "x"}})
		req := httptest.NewRequest("POST", "/api/content-bank/assets/upload", body)
		req.Header.Set("Content-Type", ct)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(401))
	})
})
