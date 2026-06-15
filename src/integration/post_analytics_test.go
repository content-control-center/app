//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/handlers"
	"github.com/ogen-app/ogen/src/jobs/queues"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/repository"
)

// memSettings is a minimal in-memory zernio.SettingsStore for driving
// the refresh processor in tests.
type memSettings struct {
	mu sync.Mutex
	kv map[string]string
}

func newMemSettings() *memSettings { return &memSettings{kv: map[string]string{}} }
func (s *memSettings) Get(_ context.Context, k string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.kv[k]
	return v, ok, nil
}
func (s *memSettings) Set(_ context.Context, k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kv[k] = v
	return nil
}
func (s *memSettings) Delete(_ context.Context, k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.kv, k)
	return nil
}

var _ = Describe("Post analytics — CON-93", Ordered, func() {
	const linkedinSqid = "AXqWG7U2qnpt"

	var (
		app           *fiber.App
		db            *bun.DB
		authCookie    *http.Cookie
		userID        string
		campaignID    string
		postRepo      repository.PostRepository
		analyticsRepo repository.PostAnalyticsRepository

		zernioCalls atomic.Int64
		zernioStub  *httptest.Server
	)

	BeforeAll(func() {
		db = mustOpenIntegrationDB()
	})

	BeforeEach(func() {
		zernioCalls.Store(0)
		// Stub Zernio: serves one analytics page with a single item that
		// matches the seeded post (publisher_post_id z-1) and one stranger
		// that Ogen doesn't own. Every request bumps the counter so the
		// read-path assertion can prove the endpoints never call Zernio.
		zernioStub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			zernioCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"analytics": []any{
					map[string]any{
						"postId":     "z-1",
						"syncStatus": "synced",
						"analytics":  map[string]any{"impressions": 1234, "reach": 1000, "likes": 56, "engagementRate": 0.07},
						"platformAnalytics": []any{
							map[string]any{"platform": "linkedin", "status": "published", "syncStatus": "synced",
								"analytics": map[string]any{"impressions": 1234, "likes": 56}},
						},
					},
					map[string]any{"postId": "z-stranger", "analytics": map[string]any{"impressions": 9}},
				},
				"pagination": map[string]any{"page": 1, "limit": 100, "total": 2, "pages": 1},
			})
		}))

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
		platformRepo := repository.NewPlatformRepository(db)
		campaignTypeRepo := repository.NewCampaignTypeRepository(db)
		campaignRepo := repository.NewCampaignRepository(db, tagRepo, platformRepo, campaignTypeRepo)
		postRepo = repository.NewPostRepository(db)
		analyticsRepo = repository.NewPostAnalyticsRepository(db)
		auth := handlers.RequireAuth(sessionRepo, "test_session")

		handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
		handlers.NewSessionsHandler(userRepo, sessionRepo, "test_session", false).Register(app)
		handlers.NewCampaignsHandler(campaignRepo, campaignTypeRepo, auth, nil, nil, nil).Register(app)

		postsHandler := handlers.NewPostsHandler(postRepo, repository.NewPostVersionRepository(db),
			repository.NewPostAssistantMessageRepository(db), platformRepo,
			repository.NewPostAttachmentRepository(db), auth, nil, nil)
		postsHandler.SetAnalyticsRepo(analyticsRepo)
		postsHandler.Register(app)
		handlers.NewAnalyticsHandler(analyticsRepo, auth).Register(app)

		body, _ := json.Marshal(fiber.Map{"name": "Admin", "email": "analytics@example.com", "password": "analytics-password"})
		req := httptest.NewRequest("POST", "/api/users", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		var createdUser models.User
		Expect(json.NewDecoder(resp.Body).Decode(&createdUser)).To(Succeed())
		userID = createdUser.ID

		loginBody, _ := json.Marshal(fiber.Map{"email": "analytics@example.com", "password": "analytics-password"})
		loginReq := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader(loginBody))
		loginReq.Header.Set("Content-Type", "application/json")
		loginResp, err := app.Test(loginReq)
		Expect(err).NotTo(HaveOccurred())
		Expect(loginResp.StatusCode).To(Equal(fiber.StatusCreated))
		authCookie = loginResp.Cookies()[0]

		cBody, _ := json.Marshal(fiber.Map{"name": "Analytics Campaign", "campaign_type_id": "Uk"})
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
		if zernioStub != nil {
			zernioStub.Close()
		}
		ctx := context.Background()
		for _, t := range []string{"post_analytics", "post_versions", "post_logs", "post_assistant_messages", "posts", "campaigns", "sessions", "users"} {
			_, _ = db.NewDelete().TableExpr(t).Where("1 = 1").Exec(ctx)
		}
	})

	seedPublishedPost := func(id, publisherPostID string) {
		published := time.Now().UTC()
		Expect(postRepo.Create(context.Background(), &models.Post{
			ID:              id,
			CampaignID:      campaignID,
			PlatformID:      linkedinSqid,
			Title:           "Post " + id,
			Content:         "content " + id,
			MediaURLs:       models.StringSlice{},
			UsedAssetIDs:    models.StringSlice{},
			Status:          models.PostStatusPublished,
			Publisher:       models.PublisherZernio,
			PublisherPostID: publisherPostID,
			CTAType:         models.CTATypeNone,
			CreatedBy:       userID,
			PublishedAt:     &published,
		})).To(Succeed())
	}

	// runRefresh runs one background refresh tick against the stub Zernio,
	// populating post_analytics from the wire — exactly as the recurring
	// queue does at runtime.
	runRefresh := func() {
		proc := &queues.RefreshZernioAnalyticsProcessor{
			Deps: queues.ZernioDeps{
				PostRepo:      postRepo,
				AnalyticsRepo: analyticsRepo,
				Client:        zernio.NewClient(zernio.StaticKey("k"), zernioStub.URL, zernio.ClientOpts{Timeout: 5 * time.Second}),
			},
			Settings:   newMemSettings(),
			WindowDays: 90,
		}
		Expect(proc.Process(context.Background(), queues.RefreshZernioAnalyticsTask{})).To(Succeed())
	}

	get := func(path string) *http.Response {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(authCookie)
		resp, err := app.Test(req)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	It("refreshes from Zernio into the DB, then serves both endpoints from the DB with no request-path Zernio call", func() {
		seedPublishedPost("post-1", "z-1")

		// Background refresh fetches from Zernio and upserts the snapshot.
		runRefresh()
		Expect(zernioCalls.Load()).To(BeNumerically(">=", 1))

		// From here on, no Zernio call may happen on the request path.
		callsAfterRefresh := zernioCalls.Load()

		// Per-post read (FR4).
		resp := get("/api/posts/post-1/analytics")
		Expect(resp.StatusCode).To(Equal(200))
		var perPost struct {
			PostID     string `json:"post_id"`
			Publisher  string `json:"publisher"`
			SyncStatus string `json:"sync_status"`
			Analytics  struct {
				Impressions int `json:"impressions"`
				Likes       int `json:"likes"`
			} `json:"analytics"`
			PlatformAnalytics []struct {
				Platform   string `json:"platform"`
				SyncStatus string `json:"sync_status"`
			} `json:"platform_analytics"`
		}
		Expect(json.NewDecoder(resp.Body).Decode(&perPost)).To(Succeed())
		Expect(perPost.PostID).To(Equal("post-1"))
		Expect(perPost.Publisher).To(Equal("zernio"))
		Expect(perPost.Analytics.Impressions).To(Equal(1234))
		Expect(perPost.Analytics.Likes).To(Equal(56))
		Expect(perPost.PlatformAnalytics).To(HaveLen(1))

		// Overview read (FR5).
		ovResp := get("/api/analytics/posts?sort_by=impressions&order=desc")
		Expect(ovResp.StatusCode).To(Equal(200))
		var overview struct {
			Items []struct {
				PostID   string `json:"post_id"`
				Platform string `json:"platform"`
			} `json:"items"`
			Pagination struct{ Total int } `json:"pagination"`
			Overview   struct {
				PostCount   int `json:"post_count"`
				Impressions int `json:"impressions"`
			} `json:"overview"`
		}
		Expect(json.NewDecoder(ovResp.Body).Decode(&overview)).To(Succeed())
		Expect(overview.Items).To(HaveLen(1))
		Expect(overview.Items[0].PostID).To(Equal("post-1"))
		Expect(overview.Items[0].Platform).To(Equal("LinkedIn"))
		Expect(overview.Overview.PostCount).To(Equal(1))
		Expect(overview.Overview.Impressions).To(Equal(1234))

		// Acceptance #7: neither read endpoint called Zernio.
		Expect(zernioCalls.Load()).To(Equal(callsAfterRefresh))
	})

	It("returns pending for a published post the refresh has not covered", func() {
		seedPublishedPost("post-2", "z-uncovered")
		resp := get("/api/posts/post-2/analytics")
		Expect(resp.StatusCode).To(Equal(200))
		var body map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		Expect(body["status"]).To(Equal("pending"))
		Expect(zernioCalls.Load()).To(Equal(int64(0)))
	})
})
