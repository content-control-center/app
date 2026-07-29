package queues_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ogen-app/ogen/src/jobs/queues"
	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/tenantctx"
)

// tctx is a tenant-scoped context, mirroring how the refresh job runs each
// tenant's sweep under tenantctx.With.
func tctx(tenantID string) context.Context {
	return tenantctx.With(context.Background(), tenantID)
}

// fakeAnalyticsRepo records inserts for assertions. Append-only in production;
// the fake keeps the latest row per post_id, which is all these tests assert on.
type fakeAnalyticsRepo struct {
	mu       sync.Mutex
	upserted map[string]*models.PostAnalytics
}

func newFakeAnalyticsRepo() *fakeAnalyticsRepo {
	return &fakeAnalyticsRepo{upserted: map[string]*models.PostAnalytics{}}
}

func (r *fakeAnalyticsRepo) Insert(_ context.Context, a *models.PostAnalytics) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *a
	r.upserted[a.PostID] = &cp
	return nil
}
func (r *fakeAnalyticsRepo) GetByPostID(context.Context, string) (*models.PostAnalytics, error) {
	return nil, nil
}
func (r *fakeAnalyticsRepo) List(context.Context, repository.PostAnalyticsListOptions) ([]repository.PostAnalyticsListItem, repository.PostAnalyticsOverview, error) {
	return nil, repository.PostAnalyticsOverview{}, nil
}

// fakeSettings is an in-memory zernio.SettingsStore keyed by (tenant, key) —
// the tenant is read from the context exactly as the real tenant-scoped
// settings repo would, so per-tenant profile ids and per-tenant refresh status
// stay isolated. Callers with no tenant in context (e.g. the bootstrap tests)
// all share the empty-tenant bucket, preserving their original behaviour.
type fakeSettings struct {
	mu sync.Mutex
	kv map[string]string
}

func newFakeSettings() *fakeSettings { return &fakeSettings{kv: map[string]string{}} }

func (s *fakeSettings) key(ctx context.Context, k string) string {
	tid, _ := tenantctx.From(ctx)
	return tid + "\x00" + k
}
func (s *fakeSettings) Get(ctx context.Context, k string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.kv[s.key(ctx, k)]
	return v, ok, nil
}
func (s *fakeSettings) Set(ctx context.Context, k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kv[s.key(ctx, k)] = v
	return nil
}
func (s *fakeSettings) Delete(ctx context.Context, k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.kv, s.key(ctx, k))
	return nil
}

// seedPublishedPost seeds a Zernio-published post owned by tenantID. seedProfile
// then records that tenant's Zernio profile so the refresh sweep queries it.
func seedPublishedPost(repo *fakePostRepo, id, publisherPostID, tenantID string) {
	repo.put(&models.Post{
		ID:              id,
		TenantScoped:    models.TenantScoped{TenantID: tenantID},
		Publisher:       models.PublisherZernio,
		PublisherPostID: publisherPostID,
		Status:          models.PostStatusPublished,
	})
}

func seedProfile(settings *fakeSettings, tenantID, profileID string) {
	_ = settings.Set(tctx(tenantID), zernio.SettingProfileID, profileID)
}

func newRefreshProcessor(stub *stubZernio, postRepo *fakePostRepo, analyticsRepo repository.PostAnalyticsRepository, settings *fakeSettings) *queues.RefreshZernioAnalyticsProcessor {
	return &queues.RefreshZernioAnalyticsProcessor{
		Deps: queues.ZernioDeps{
			PostRepo:      postRepo,
			AnalyticsRepo: analyticsRepo,
			Client:        zernio.NewClient(zernio.StaticKey("k"), stub.URL, zernio.ClientOpts{Timeout: 5 * time.Second}),
			// Per-profile: resolve each tenant's Zernio profile from settings,
			// scoped by the ctx tenant — exactly the server's wiring.
			ProfileID: func(ctx context.Context) (string, error) {
				v, _, err := settings.Get(ctx, zernio.SettingProfileID)
				return v, err
			},
		},
		Settings: settings,
		// Everything else left zero: no self-reschedule in unit tests.
	}
}

func TestRefreshMatchesAndUpsertsOnlyKnownPosts(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()

	stub.handle("GET", "/analytics", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("source"); got != "late" {
			t.Errorf("source: got %q want late", got)
		}
		if got := r.URL.Query().Get("profileId"); got != "prof-1" {
			t.Errorf("profileId: got %q want prof-1 (per-profile scoping)", got)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"analytics": []any{
				map[string]any{
					"postId":     "z-1",
					"syncStatus": "synced",
					"analytics":  map[string]any{"impressions": 1234, "likes": 56, "engagementRate": 0.07},
					"platformAnalytics": []any{
						map[string]any{"platform": "linkedin", "status": "published", "syncStatus": "synced",
							"analytics": map[string]any{"impressions": 1234, "likes": 56}},
					},
				},
				// Not owned by Ogen → must be ignored.
				map[string]any{"postId": "z-stranger", "analytics": map[string]any{"impressions": 9}},
			},
			"pagination": map[string]any{"page": 1, "limit": 100, "total": 2, "pages": 1},
		})
	})

	postRepo := newFakePostRepo()
	seedPublishedPost(postRepo, "post-1", "z-1", "t1")
	analyticsRepo := newFakeAnalyticsRepo()
	settings := newFakeSettings()
	seedProfile(settings, "t1", "prof-1")

	proc := newRefreshProcessor(stub, postRepo, analyticsRepo, settings)
	if err := proc.Process(context.Background(), queues.RefreshZernioAnalyticsTask{}); err != nil {
		t.Fatalf("process: %v", err)
	}

	if len(analyticsRepo.upserted) != 1 {
		t.Fatalf("upserts: got %d want 1", len(analyticsRepo.upserted))
	}
	got := analyticsRepo.upserted["post-1"]
	if got == nil {
		t.Fatalf("post-1 not upserted")
	}
	if got.PublisherPostID != "z-1" || got.Impressions != 1234 || got.Likes != 56 {
		t.Errorf("snapshot wrong: %+v", got)
	}
	if got.EngagementRate != 0.07 {
		t.Errorf("engagement_rate: got %v", got.EngagementRate)
	}
	if len(got.PlatformAnalytics) != 1 || got.PlatformAnalytics[0].Platform != "linkedin" {
		t.Errorf("platform breakdown not mapped: %+v", got.PlatformAnalytics)
	}
	if got.SyncStatus != "synced" {
		t.Errorf("sync_status: got %q", got.SyncStatus)
	}
	// Health is recorded under the tenant's context (per-profile).
	if v, _, _ := settings.Get(tctx("t1"), zernio.SettingAnalyticsLastRefreshStatus); v != zernio.SyncStatusOK {
		t.Errorf("refresh status: got %q want ok", v)
	}
	if v, _, _ := settings.Get(tctx("t1"), zernio.SettingAnalyticsLastRefreshAt); v == "" {
		t.Errorf("last_refresh_at not written")
	}
}

func TestRefreshPagesThroughAllPages(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()

	stub.handle("GET", "/analytics", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		switch page {
		case "1", "":
			writeJSON(w, http.StatusOK, map[string]any{
				"analytics":  []any{map[string]any{"postId": "z-1", "analytics": map[string]any{"impressions": 10}}},
				"pagination": map[string]any{"page": 1, "limit": 100, "total": 2, "pages": 2},
			})
		case "2":
			writeJSON(w, http.StatusOK, map[string]any{
				"analytics":  []any{map[string]any{"postId": "z-2", "analytics": map[string]any{"impressions": 20}}},
				"pagination": map[string]any{"page": 2, "limit": 100, "total": 2, "pages": 2},
			})
		default:
			t.Errorf("unexpected page %q", page)
		}
	})

	postRepo := newFakePostRepo()
	seedPublishedPost(postRepo, "post-1", "z-1", "t1")
	seedPublishedPost(postRepo, "post-2", "z-2", "t1")
	analyticsRepo := newFakeAnalyticsRepo()
	settings := newFakeSettings()
	seedProfile(settings, "t1", "prof-1")

	proc := newRefreshProcessor(stub, postRepo, analyticsRepo, settings)
	if err := proc.Process(context.Background(), queues.RefreshZernioAnalyticsTask{}); err != nil {
		t.Fatalf("process: %v", err)
	}
	if len(analyticsRepo.upserted) != 2 {
		t.Fatalf("upserts: got %d want 2", len(analyticsRepo.upserted))
	}
	if analyticsRepo.upserted["post-2"].Impressions != 20 {
		t.Errorf("page 2 not applied: %+v", analyticsRepo.upserted["post-2"])
	}
}

// TestRefreshSweepsPerProfile is the core per-profile assertion: two tenants,
// each with its own Zernio profile, are swept independently — every fetch
// carries that tenant's profileId, and a post is only upserted under the tenant
// that owns it.
func TestRefreshSweepsPerProfile(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()

	var mu sync.Mutex
	seenProfiles := map[string]bool{}
	// Each profile returns only its own post — proving the fetch is scoped.
	postForProfile := map[string]string{"prof-A": "z-a", "prof-B": "z-b"}

	stub.handle("GET", "/analytics", func(w http.ResponseWriter, r *http.Request) {
		prof := r.URL.Query().Get("profileId")
		mu.Lock()
		seenProfiles[prof] = true
		mu.Unlock()
		postID := postForProfile[prof]
		if postID == "" {
			writeJSON(w, http.StatusOK, map[string]any{"analytics": []any{}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"analytics":  []any{map[string]any{"postId": postID, "analytics": map[string]any{"impressions": 5}}},
			"pagination": map[string]any{"page": 1, "limit": 100, "total": 1, "pages": 1},
		})
	})

	postRepo := newFakePostRepo()
	seedPublishedPost(postRepo, "post-a", "z-a", "tenant-A")
	seedPublishedPost(postRepo, "post-b", "z-b", "tenant-B")
	analyticsRepo := newFakeAnalyticsRepo()
	settings := newFakeSettings()
	seedProfile(settings, "tenant-A", "prof-A")
	seedProfile(settings, "tenant-B", "prof-B")

	proc := newRefreshProcessor(stub, postRepo, analyticsRepo, settings)
	if err := proc.Process(context.Background(), queues.RefreshZernioAnalyticsTask{}); err != nil {
		t.Fatalf("process: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !seenProfiles["prof-A"] || !seenProfiles["prof-B"] {
		t.Errorf("expected both profiles queried, got %v", seenProfiles)
	}
	if len(analyticsRepo.upserted) != 2 {
		t.Fatalf("upserts: got %d want 2 (%+v)", len(analyticsRepo.upserted), analyticsRepo.upserted)
	}
	if analyticsRepo.upserted["post-a"] == nil || analyticsRepo.upserted["post-b"] == nil {
		t.Errorf("both tenants' posts should be upserted: %+v", analyticsRepo.upserted)
	}
	// Each tenant got its own health record.
	if v, _, _ := settings.Get(tctx("tenant-A"), zernio.SettingAnalyticsLastRefreshStatus); v != zernio.SyncStatusOK {
		t.Errorf("tenant-A status: got %q want ok", v)
	}
	if v, _, _ := settings.Get(tctx("tenant-B"), zernio.SettingAnalyticsLastRefreshStatus); v != zernio.SyncStatusOK {
		t.Errorf("tenant-B status: got %q want ok", v)
	}
}

// TestRefreshSkipsTenantWithoutProfile: a tenant that owns Zernio-published
// posts but has no profile id recorded is skipped — no fetch, no upsert — rather
// than falling back to an unscoped cross-profile call.
func TestRefreshSkipsTenantWithoutProfile(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()

	var called bool
	stub.handle("GET", "/analytics", func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, http.StatusOK, map[string]any{"analytics": []any{}})
	})

	postRepo := newFakePostRepo()
	seedPublishedPost(postRepo, "post-1", "z-1", "t1")
	analyticsRepo := newFakeAnalyticsRepo()
	settings := newFakeSettings() // no profile seeded for t1

	proc := newRefreshProcessor(stub, postRepo, analyticsRepo, settings)
	if err := proc.Process(context.Background(), queues.RefreshZernioAnalyticsTask{}); err != nil {
		t.Fatalf("process: %v", err)
	}
	if called {
		t.Errorf("analytics endpoint should not be called for a profile-less tenant")
	}
	if len(analyticsRepo.upserted) != 0 {
		t.Errorf("upserts: got %d want 0", len(analyticsRepo.upserted))
	}
}

func TestRefreshRateLimitRecordedChainContinues(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("GET", "/analytics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "slow down"})
	})

	postRepo := newFakePostRepo()
	seedPublishedPost(postRepo, "post-1", "z-1", "t1")
	settings := newFakeSettings()
	seedProfile(settings, "t1", "prof-1")
	proc := newRefreshProcessor(stub, postRepo, newFakeAnalyticsRepo(), settings)

	// A 429 is recorded but does NOT propagate — Process always returns nil
	// and self-reschedules so the recurring chain can't die or fan out (the
	// reschedule itself is a no-op here: no River client in ctx). The
	// failure is still recorded for operators, per tenant.
	err := proc.Process(context.Background(), queues.RefreshZernioAnalyticsTask{})
	if err != nil {
		t.Fatalf("Process should return nil so the chain self-reschedules, got %v", err)
	}
	if v, _, _ := settings.Get(tctx("t1"), zernio.SettingAnalyticsLastRefreshStatus); v == zernio.SyncStatusOK || v == "" {
		t.Errorf("status should record the error, got %q", v)
	}
}

func TestRefreshLegacy402IsTerminalNoRetry(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("GET", "/analytics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "add-on required"})
	})

	postRepo := newFakePostRepo()
	seedPublishedPost(postRepo, "post-1", "z-1", "t1")
	settings := newFakeSettings()
	seedProfile(settings, "t1", "prof-1")
	proc := newRefreshProcessor(stub, postRepo, newFakeAnalyticsRepo(), settings)

	// Legacy 402 is handled (logged + recorded), not retried → Process nil.
	if err := proc.Process(context.Background(), queues.RefreshZernioAnalyticsTask{}); err != nil {
		t.Fatalf("legacy 402 should be handled terminally, got %v", err)
	}
}

func TestRefresh401IsTerminalNoRetry(t *testing.T) {
	stub := newStubZernio()
	defer stub.Close()
	stub.handle("GET", "/analytics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	})

	postRepo := newFakePostRepo()
	seedPublishedPost(postRepo, "post-1", "z-1", "t1")
	settings := newFakeSettings()
	seedProfile(settings, "t1", "prof-1")
	proc := newRefreshProcessor(stub, postRepo, newFakeAnalyticsRepo(), settings)

	if err := proc.Process(context.Background(), queues.RefreshZernioAnalyticsTask{}); err != nil {
		t.Fatalf("401 should be handled terminally (account worker owns auth), got %v", err)
	}
}
