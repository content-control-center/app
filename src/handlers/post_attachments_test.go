package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
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

// ── Image fixtures ──────────────────────────────────────────────────────────

func minimalJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.Bytes()
}

func staticGIF() []byte {
	pal := color.Palette{color.Black, color.White}
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), pal)
	var buf bytes.Buffer
	_ = gif.Encode(&buf, img, nil)
	return buf.Bytes()
}

func animatedGIF() []byte {
	pal := color.Palette{color.Black, color.White}
	frame := image.NewPaletted(image.Rect(0, 0, 1, 1), pal)
	g := &gif.GIF{
		Image: []*image.Paletted{frame, frame},
		Delay: []int{10, 10},
	}
	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, g)
	return buf.Bytes()
}

// minimalPDF returns the bytes of a tiny structurally-valid 1-page PDF
// that ledongthuc/pdf.PageCount can parse. Used by CON-75 PDF tests.
func minimalPDF() []byte {
	return minimalPDFWithPages(1)
}

// minimalPDFWithPages builds a structurally-valid PDF with n pages.
// Cross-reference offsets are computed against the actual byte layout
// so the file round-trips through pdfprobe.
func minimalPDFWithPages(n int) []byte {
	if n < 1 {
		n = 1
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	offsets := make([]int, 0, 2+n)

	// Catalog
	offsets = append(offsets, buf.Len())
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// Pages tree — children are 3..(2+n)
	offsets = append(offsets, buf.Len())
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [")
	for i := 0; i < n; i++ {
		if i > 0 {
			buf.WriteString(" ")
		}
		buf.WriteString(fmt.Sprintf("%d 0 R", 3+i))
	}
	buf.WriteString(fmt.Sprintf("] /Count %d >>\nendobj\n", n))

	// Pages
	for i := 0; i < n; i++ {
		offsets = append(offsets, buf.Len())
		buf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n", 3+i))
	}

	// xref
	xrefStart := buf.Len()
	totalObjs := 2 + n // catalog + pages tree + n page objects
	buf.WriteString(fmt.Sprintf("xref\n0 %d\n", totalObjs+1))
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}

	// Trailer
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\n", totalObjs+1))
	buf.WriteString(fmt.Sprintf("startxref\n%d\n", xrefStart))
	buf.WriteString("%%EOF\n")
	return buf.Bytes()
}

// multipartBodyAttachment builds a multipart/form-data body for the
// post-attachments handler (same field name as ImagesHandler so the
// helper can be shared if needed in the future).
func multipartBodyAttachment(filename string, content []byte) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", filename)
	_, _ = fw.Write(content)
	w.Close()
	return &buf, w.FormDataContentType()
}

var _ = Describe("PostAttachmentsHandler", Ordered, func() {
	var (
		app        *fiber.App
		db         *bun.DB
		authCookie *http.Cookie
		campaignID string
		stub       *stubStorage
	)

	const linkedinPlatformID = "AXqWG7U2qnpt"
	const xPlatformID = "81mUCmc2xsKd"

	BeforeAll(func() {
		db = mustOpenTestDBWithMigrations()
	})

	BeforeEach(func() {
		stub = &stubStorage{returnURL: "https://pub.example.com/x", objects: map[string][]byte{}}

		app = fiber.New(fiber.Config{
			BodyLimit: 60 << 20,
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
		campaignTypeRepo := repository.NewCampaignTypeRepository(db)
		campaignRepo := repository.NewCampaignRepository(db, tagRepo, repository.NewPlatformRepository(db), campaignTypeRepo)
		postRepo := repository.NewPostRepository(db)
		postAttRepo := repository.NewPostAttachmentRepository(db)
		auth := handlers.RequireAuth(sessionRepo, testCookieName)

		handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, testCookieName, false).Register(app)
		handlers.NewCampaignsHandler(campaignRepo, campaignTypeRepo, auth, nil, nil, nil).Register(app)
		postVersionRepo := repository.NewPostVersionRepository(db)
		postMessageRepo := repository.NewPostAssistantMessageRepository(db)
		handlers.NewPostsHandler(postRepo, postVersionRepo, postMessageRepo, repository.NewPlatformRepository(db), postAttRepo, auth, nil, nil).Register(app)
		handlers.NewPostAttachmentsHandler(postAttRepo, postRepo, stub, auth).Register(app)

		body, _ := json.Marshal(fiber.Map{"name": "Admin", "email": "att@example.com", "password": "att-password"})
		req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

		loginBody, _ := json.Marshal(fiber.Map{"email": "att@example.com", "password": "att-password"})
		loginReq := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(loginBody))
		loginReq.Header.Set("Content-Type", "application/json")
		loginResp, err := app.Test(loginReq)
		Expect(err).NotTo(HaveOccurred())
		Expect(loginResp.StatusCode).To(Equal(fiber.StatusCreated))
		cookies := loginResp.Cookies()
		Expect(cookies).To(HaveLen(1))
		authCookie = cookies[0]

		cBody, _ := json.Marshal(fiber.Map{"name": "Att Campaign", "campaign_type_id": "Uk"})
		cReq := httptest.NewRequest("POST", "/api/campaigns", bytes.NewReader(cBody))
		cReq.Header.Set("Content-Type", "application/json")
		cReq.AddCookie(authCookie)
		cResp, err := app.Test(cReq)
		Expect(err).NotTo(HaveOccurred())
		Expect(cResp.StatusCode).To(Equal(fiber.StatusCreated))
		var camp models.Campaign
		Expect(json.NewDecoder(cResp.Body).Decode(&camp)).To(Succeed())
		campaignID = camp.ID
	})

	AfterEach(func() {
		ctx := context.Background()
		_, _ = db.NewDelete().TableExpr("post_attachments").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("post_assistant_messages").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("post_versions").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("posts").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("campaigns").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("sessions").Where("1 = 1").Exec(ctx)
		_, _ = db.NewDelete().TableExpr("users").Where("1 = 1").Exec(ctx)
	})

	// helpers
	createPostWithPlatform := func(platformID string) string {
		body, _ := json.Marshal(fiber.Map{
			"campaign_id":        campaignID,
			"platform_id":        platformID,
			"platform_post_type": "image-post",
			"title":              "Att Post",
		})
		req := httptest.NewRequest("POST", "/api/posts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(authCookie)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))
		var p models.Post
		Expect(json.NewDecoder(resp.Body).Decode(&p)).To(Succeed())
		return p.ID
	}

	uploadPNG := func(postID string, content []byte) (*http.Response, error) {
		body, ct := multipartBodyAttachment("image.png", content)
		req := httptest.NewRequest("POST", "/api/posts/"+postID+"/attachments", body)
		req.Header.Set("Content-Type", ct)
		req.AddCookie(authCookie)
		return app.Test(req, 30000)
	}

	// ── POST /api/posts/:post_id/attachments ─────────────────────────────────

	Describe("POST /api/posts/:post_id/attachments", func() {
		Context("when not authenticated", func() {
			It("returns 401", func() {
				body, ct := multipartBodyAttachment("img.png", minimalPNG())
				req := httptest.NewRequest("POST", "/api/posts/anything/attachments", body)
				req.Header.Set("Content-Type", ct)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when authenticated", func() {
			It("returns 404 when the post does not exist", func() {
				body, ct := multipartBodyAttachment("img.png", minimalPNG())
				req := httptest.NewRequest("POST", "/api/posts/nope/attachments", body)
				req.Header.Set("Content-Type", ct)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})

			It("creates an attachment for a valid PNG and returns 201 with metadata", func() {
				postID := createPostWithPlatform(linkedinPlatformID)
				resp, err := uploadPNG(postID, minimalPNG())
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

				var got map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got["id"]).NotTo(BeEmpty())
				Expect(got["post_id"]).To(Equal(postID))
				Expect(got["mime_type"]).To(Equal("image/png"))
				Expect(got["position"]).To(BeEquivalentTo(0))
				Expect(got["width"]).To(BeEquivalentTo(1))
				Expect(got["height"]).To(BeEquivalentTo(1))
				Expect(got["is_animated"]).To(BeFalse())
				Expect(got["checksum_sha256"]).NotTo(BeEmpty())
				Expect(got["s3_key"]).To(ContainSubstring("post-attachments/" + postID + "/"))
				Expect(got["presigned_url"]).To(ContainSubstring("/signed/"))
				// LinkedIn allows PNG → no platform_validation errors.
				Expect(got["platform_validation"]).To(BeNil())
			})

			It("auto-increments position for sequential uploads", func() {
				postID := createPostWithPlatform(linkedinPlatformID)
				_, err := uploadPNG(postID, minimalPNG())
				Expect(err).NotTo(HaveOccurred())
				resp, err := uploadPNG(postID, minimalJPEG())
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

				var got map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got["position"]).To(BeEquivalentTo(1))
			})

			It("rejects a non-image MIME with 415", func() {
				postID := createPostWithPlatform(linkedinPlatformID)
				body, ct := multipartBodyAttachment("doc.txt", []byte("hello world this is plain text padding to defeat sniffer"))
				req := httptest.NewRequest("POST", "/api/posts/"+postID+"/attachments", body)
				req.Header.Set("Content-Type", ct)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(415))
			})

			It("magic bytes win over the .png filename — a real PDF is routed to the PDF path even when named .png", func() {
				postID := createPostWithPlatform(linkedinPlatformID)
				body, ct := multipartBodyAttachment("evil.png", minimalPDF())
				req := httptest.NewRequest("POST", "/api/posts/"+postID+"/attachments", body)
				req.Header.Set("Content-Type", ct)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

				var got map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got["mime_type"]).To(Equal("application/pdf"))
				Expect(got["s3_key"]).To(ContainSubstring(".pdf"))
			})

			It("rejects a malformed PDF — magic bytes alone are not enough", func() {
				postID := createPostWithPlatform(linkedinPlatformID)
				body, ct := multipartBodyAttachment("doc.pdf", []byte("%PDF-1.4\n%fake pdf payload"))
				req := httptest.NewRequest("POST", "/api/posts/"+postID+"/attachments", body)
				req.Header.Set("Content-Type", ct)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})

			It("returns platform_validation warnings for an animated GIF on LinkedIn (animated_gif_supported=false)", func() {
				postID := createPostWithPlatform(linkedinPlatformID)
				body, ct := multipartBodyAttachment("anim.gif", animatedGIF())
				req := httptest.NewRequest("POST", "/api/posts/"+postID+"/attachments", body)
				req.Header.Set("Content-Type", ct)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

				var got map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got["is_animated"]).To(BeTrue())
				warnings := got["platform_validation"].([]any)
				Expect(warnings).NotTo(BeEmpty())
				first := warnings[0].(map[string]any)
				Expect(first["rule"]).To(Equal("animated_gif_supported"))
			})

			It("accepts an animated GIF without warnings on X (animated_gif_supported=true)", func() {
				postID := createPostWithPlatform(xPlatformID)
				body, ct := multipartBodyAttachment("anim.gif", animatedGIF())
				req := httptest.NewRequest("POST", "/api/posts/"+postID+"/attachments", body)
				req.Header.Set("Content-Type", ct)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

				var got map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				Expect(got["is_animated"]).To(BeTrue())
				Expect(got["platform_validation"]).To(BeNil())
			})

			It("returns 409 when the post is published", func() {
				postID := createPostWithPlatform(linkedinPlatformID)
				ctx := context.Background()
				_, err := db.NewUpdate().Model((*models.Post)(nil)).
					Set("status = ?", models.PostStatusPublished).
					Where("id = ?", postID).Exec(ctx)
				Expect(err).NotTo(HaveOccurred())

				resp, err := uploadPNG(postID, minimalPNG())
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(409))
			})

			It("rejects a static GIF on Instagram with platform_validation (gif not in allowed_formats)", func() {
				postID := createPostWithPlatform("rzgpTkARLH0L") // Instagram
				body, ct := multipartBodyAttachment("static.gif", staticGIF())
				req := httptest.NewRequest("POST", "/api/posts/"+postID+"/attachments", body)
				req.Header.Set("Content-Type", ct)
				req.AddCookie(authCookie)
				resp, err := app.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

				var got map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
				warnings := got["platform_validation"].([]any)
				Expect(warnings).NotTo(BeEmpty())
				Expect(warnings[0].(map[string]any)["rule"]).To(Equal("allowed_formats"))
			})

			// ── CON-75: PDF attachments ──────────────────────────────────────
			Context("with a PDF file", func() {
				uploadPDF := func(postID, filename string, content []byte) (*http.Response, error) {
					body, ct := multipartBodyAttachment(filename, content)
					req := httptest.NewRequest("POST", "/api/posts/"+postID+"/attachments", body)
					req.Header.Set("Content-Type", ct)
					req.AddCookie(authCookie)
					return app.Test(req, 30000)
				}

				It("creates a PDF attachment for LinkedIn — page_count is set, no warnings", func() {
					postID := createPostWithPlatform(linkedinPlatformID)
					resp, err := uploadPDF(postID, "carousel.pdf", minimalPDFWithPages(3))
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

					var got map[string]any
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					Expect(got["mime_type"]).To(Equal("application/pdf"))
					Expect(got["page_count"]).To(BeEquivalentTo(3))
					Expect(got["s3_key"]).To(ContainSubstring(".pdf"))
					Expect(got["platform_validation"]).To(BeNil())
					Expect(got["width"]).To(BeEquivalentTo(0))
					Expect(got["height"]).To(BeEquivalentTo(0))
					Expect(got["is_animated"]).To(BeFalse())
				})

				It("returns a pdf_not_supported soft warning on Instagram", func() {
					postID := createPostWithPlatform("rzgpTkARLH0L") // Instagram
					resp, err := uploadPDF(postID, "doc.pdf", minimalPDF())
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

					var got map[string]any
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					warnings := got["platform_validation"].([]any)
					Expect(warnings).NotTo(BeEmpty())
					Expect(warnings[0].(map[string]any)["rule"]).To(Equal("pdf_not_supported"))
				})

				It("surfaces a max_pages soft warning when the PDF exceeds the platform cap", func() {
					postID := createPostWithPlatform(linkedinPlatformID)

					// Tighten LinkedIn's cap so the test PDF stays small.
					ctx := context.Background()
					_, err := db.NewUpdate().
						Table("platforms").
						Set("pdf_constraints = json_set(pdf_constraints, '$.max_pages', 1)").
						Where("id = ?", linkedinPlatformID).
						Exec(ctx)
					Expect(err).NotTo(HaveOccurred())

					resp, err := uploadPDF(postID, "long.pdf", minimalPDFWithPages(2))
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

					var got map[string]any
					Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
					warnings := got["platform_validation"].([]any)
					Expect(warnings).NotTo(BeEmpty())
					rules := []string{}
					for _, w := range warnings {
						rules = append(rules, w.(map[string]any)["rule"].(string))
					}
					Expect(rules).To(ContainElement("max_pages"))
				})

				It("surfaces a mix warning when a post has both an image and a PDF", func() {
					postID := createPostWithPlatform(linkedinPlatformID)

					_, err := uploadPNG(postID, minimalPNG())
					Expect(err).NotTo(HaveOccurred())
					_, err = uploadPDF(postID, "doc.pdf", minimalPDF())
					Expect(err).NotTo(HaveOccurred())

					listReq := httptest.NewRequest("GET", "/api/posts/"+postID+"/attachments", nil)
					listReq.AddCookie(authCookie)
					listResp, err := app.Test(listReq)
					Expect(err).NotTo(HaveOccurred())
					Expect(listResp.StatusCode).To(Equal(200))

					var got map[string]any
					Expect(json.NewDecoder(listResp.Body).Decode(&got)).To(Succeed())
					warnings := got["platform_validation"].([]any)
					Expect(warnings).NotTo(BeEmpty())
					rules := []string{}
					for _, w := range warnings {
						rules = append(rules, w.(map[string]any)["rule"].(string))
					}
					Expect(rules).To(ContainElement("attachment_kind_mix"))
				})

				It("Delete also removes the thumbnail key from storage when one was rendered", func() {
					postID := createPostWithPlatform(linkedinPlatformID)
					resp, err := uploadPDF(postID, "doc.pdf", minimalPDF())
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(fiber.StatusCreated))

					var att map[string]any
					Expect(json.NewDecoder(resp.Body).Decode(&att)).To(Succeed())
					id := att["id"].(string)
					key := att["s3_key"].(string)
					thumbKey, _ := att["thumbnail_s3_key"].(string)

					delReq := httptest.NewRequest("DELETE", "/api/posts/"+postID+"/attachments/"+id, nil)
					delReq.AddCookie(authCookie)
					delResp, err := app.Test(delReq)
					Expect(err).NotTo(HaveOccurred())
					Expect(delResp.StatusCode).To(Equal(204))

					Expect(stub.objects).NotTo(HaveKey(key))
					if thumbKey != "" {
						Expect(stub.objects).NotTo(HaveKey(thumbKey))
					}
				})
			})
		})
	})

	// ── GET /api/posts/:post_id/attachments ──────────────────────────────────

	Describe("GET /api/posts/:post_id/attachments", func() {
		It("returns 401 when not authenticated", func() {
			req := httptest.NewRequest("GET", "/api/posts/x/attachments", nil)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(401))
		})

		It("returns an empty list for a post with no attachments", func() {
			postID := createPostWithPlatform(linkedinPlatformID)
			req := httptest.NewRequest("GET", "/api/posts/"+postID+"/attachments", nil)
			req.AddCookie(authCookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var got map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
			atts := got["attachments"].([]any)
			Expect(atts).To(BeEmpty())
		})

		It("returns attachments ordered by position", func() {
			postID := createPostWithPlatform(linkedinPlatformID)
			_, err := uploadPNG(postID, minimalPNG())
			Expect(err).NotTo(HaveOccurred())
			_, err = uploadPNG(postID, minimalJPEG())
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("GET", "/api/posts/"+postID+"/attachments", nil)
			req.AddCookie(authCookie)
			resp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(200))

			var got map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&got)).To(Succeed())
			atts := got["attachments"].([]any)
			Expect(atts).To(HaveLen(2))
			Expect(atts[0].(map[string]any)["position"]).To(BeEquivalentTo(0))
			Expect(atts[1].(map[string]any)["position"]).To(BeEquivalentTo(1))
		})
	})

	// ── PATCH /api/posts/:post_id/attachments/:id ───────────────────────────

	Describe("PATCH /api/posts/:post_id/attachments/:id", func() {
		It("updates the position", func() {
			postID := createPostWithPlatform(linkedinPlatformID)
			resp, err := uploadPNG(postID, minimalPNG())
			Expect(err).NotTo(HaveOccurred())
			var att map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&att)).To(Succeed())
			id := att["id"].(string)

			body, _ := json.Marshal(fiber.Map{"position": 5})
			req := httptest.NewRequest("PATCH", "/api/posts/"+postID+"/attachments/"+id, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(authCookie)
			pResp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(pResp.StatusCode).To(Equal(200))

			var got map[string]any
			Expect(json.NewDecoder(pResp.Body).Decode(&got)).To(Succeed())
			Expect(got["position"]).To(BeEquivalentTo(5))
		})

		It("returns 400 for a negative position", func() {
			postID := createPostWithPlatform(linkedinPlatformID)
			resp, err := uploadPNG(postID, minimalPNG())
			Expect(err).NotTo(HaveOccurred())
			var att map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&att)).To(Succeed())
			id := att["id"].(string)

			body, _ := json.Marshal(fiber.Map{"position": -1})
			req := httptest.NewRequest("PATCH", "/api/posts/"+postID+"/attachments/"+id, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(authCookie)
			pResp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(pResp.StatusCode).To(Equal(400))
		})

		It("returns 404 when the attachment belongs to a different post", func() {
			postA := createPostWithPlatform(linkedinPlatformID)
			postB := createPostWithPlatform(linkedinPlatformID)
			resp, err := uploadPNG(postA, minimalPNG())
			Expect(err).NotTo(HaveOccurred())
			var att map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&att)).To(Succeed())
			id := att["id"].(string)

			body, _ := json.Marshal(fiber.Map{"position": 2})
			req := httptest.NewRequest("PATCH", "/api/posts/"+postB+"/attachments/"+id, bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(authCookie)
			pResp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(pResp.StatusCode).To(Equal(404))
		})
	})

	// ── DELETE /api/posts/:post_id/attachments/:id ──────────────────────────

	Describe("DELETE /api/posts/:post_id/attachments/:id", func() {
		It("removes the row and the S3 object", func() {
			postID := createPostWithPlatform(linkedinPlatformID)
			resp, err := uploadPNG(postID, minimalPNG())
			Expect(err).NotTo(HaveOccurred())
			var att map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&att)).To(Succeed())
			id := att["id"].(string)
			key := att["s3_key"].(string)
			Expect(stub.objects).To(HaveKey(key))

			req := httptest.NewRequest("DELETE", "/api/posts/"+postID+"/attachments/"+id, nil)
			req.AddCookie(authCookie)
			dResp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(dResp.StatusCode).To(Equal(204))

			Expect(stub.objects).NotTo(HaveKey(key))

			// row gone
			listReq := httptest.NewRequest("GET", "/api/posts/"+postID+"/attachments", nil)
			listReq.AddCookie(authCookie)
			listResp, err := app.Test(listReq)
			Expect(err).NotTo(HaveOccurred())
			var got map[string]any
			Expect(json.NewDecoder(listResp.Body).Decode(&got)).To(Succeed())
			Expect(got["attachments"].([]any)).To(BeEmpty())
		})

		It("returns 409 when the post is published", func() {
			postID := createPostWithPlatform(linkedinPlatformID)
			resp, err := uploadPNG(postID, minimalPNG())
			Expect(err).NotTo(HaveOccurred())
			var att map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&att)).To(Succeed())
			id := att["id"].(string)

			ctx := context.Background()
			_, err = db.NewUpdate().Model((*models.Post)(nil)).
				Set("status = ?", models.PostStatusPublished).
				Where("id = ?", postID).Exec(ctx)
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("DELETE", "/api/posts/"+postID+"/attachments/"+id, nil)
			req.AddCookie(authCookie)
			dResp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(dResp.StatusCode).To(Equal(409))
		})

		It("cascades when the parent post is deleted", func() {
			postID := createPostWithPlatform(linkedinPlatformID)
			_, err := uploadPNG(postID, minimalPNG())
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("DELETE", "/api/posts/"+postID, nil)
			req.AddCookie(authCookie)
			dResp, err := app.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(dResp.StatusCode).To(Equal(204))

			// Direct DB query — handler list would also 404 since the post is gone.
			ctx := context.Background()
			n, err := db.NewSelect().
				Model((*models.PostAttachment)(nil)).
				Where("post_id = ?", postID).
				Count(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(0))
		})
	})

	// ── png helper kept here so it stays in the same package ─────────────────

	Describe("png decoder fixture", func() {
		It("emits a parseable PNG", func() {
			cfg, _, err := image.Decode(bytes.NewReader(minimalPNG()))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())

			cfg2, _, err := image.Decode(bytes.NewReader(staticGIF()))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg2).NotTo(BeNil())

			// Confirm static GIF is single-frame for the validator.
			g, err := gif.DecodeAll(bytes.NewReader(staticGIF()))
			Expect(err).NotTo(HaveOccurred())
			Expect(g.Image).To(HaveLen(1))
		})

		It("emits a parseable JPEG", func() {
			cfg, _, err := image.Decode(bytes.NewReader(minimalJPEG()))
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())
		})

		It("emits an animated GIF", func() {
			g, err := gif.DecodeAll(bytes.NewReader(animatedGIF()))
			Expect(err).NotTo(HaveOccurred())
			Expect(len(g.Image)).To(BeNumerically(">", 1))
		})

		It("png helper is reachable from this package", func() {
			Expect(png.Encode(&bytes.Buffer{}, image.NewRGBA(image.Rect(0, 0, 1, 1)))).To(Succeed())
		})
	})
})
