package server

import (
	"context"
	"database/sql"
	"errors"
	"expvar"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/uptrace/bun"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/embedding"
	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/genkit/flows/content_plan"
	"github.com/ogen-app/ogen/src/genkit/flows/enrich_brief"
	"github.com/ogen-app/ogen/src/genkit/flows/post_assistant"
	"github.com/ogen-app/ogen/src/genkit/flows/post_quality"
	"github.com/ogen-app/ogen/src/handlers"
	"github.com/ogen-app/ogen/src/jobs"
	"github.com/ogen-app/ogen/src/jobs/queues"
	"github.com/ogen-app/ogen/src/post_actions/clone"
	"github.com/ogen-app/ogen/src/post_actions/restore"
	"github.com/ogen-app/ogen/src/post_actions/schedule"
	"github.com/ogen-app/ogen/src/publishers"
	pubzernio "github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/secrets"
	"github.com/ogen-app/ogen/src/storage"
)

// TODO: refactor this function
func New(ctx context.Context, db *bun.DB, cfg *config.Config, secretStore secrets.Store) (*fiber.App, error) {
	app := fiber.New(fiber.Config{
		ErrorHandler: defaultErrorHandler,
		// WriteTimeout 0 disables the per-response write deadline so that SSE
		// streams (e.g. /generate-draft) are not forcibly closed mid-flight.
		WriteTimeout: 0,
		// Allow batched markdown uploads (up to 10 MB per file).
		BodyLimit: 100 << 20,
	})

	app.Use(recover.New())
	app.Use(logger.New())

	// CORS for the decoupled UI (CON-98). When the SPA is served from a
	// different origin, the configured UI origin(s) must be allowed to call
	// the API with credentials so the c3_session cookie is accepted. Empty
	// CORSAllowedOrigins (same-origin dev, or a UI that reverse-proxies /api)
	// leaves CORS off entirely.
	if cfg.CORSAllowedOrigins != "" {
		app.Use(cors.New(cors.Config{
			AllowOrigins:     cfg.CORSAllowedOrigins,
			AllowCredentials: true,
			AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
			AllowHeaders:     "Content-Type",
		}))
	}

	// API routes
	userRepo := repository.NewUserRepository(db)
	tenantRepo := repository.NewTenantRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	tagRepo := repository.NewTagRepository(db)
	chunksRepo := repository.NewAssetChunksRepository(db)
	assetFileRepo := repository.NewAssetFileRepository(db)
	pieceRepo := repository.NewAssetRepository(db, tagRepo, assetFileRepo)
	platformRepo := repository.NewPlatformRepository(db)
	campaignTypeRepo := repository.NewCampaignTypeRepository(db)
	campaignRepo := repository.NewCampaignRepository(db, tagRepo, platformRepo, campaignTypeRepo)
	postRepo := repository.NewPostRepository(db)
	postVersionRepo := repository.NewPostVersionRepository(db)
	postMessageRepo := repository.NewPostAssistantMessageRepository(db)
	postAttachmentRepo := repository.NewPostAttachmentRepository(db)
	postLogRepo := repository.NewPostLogRepository(db)
	postEvaluationRepo := repository.NewPostEvaluationRepository(db)
	postAnalyticsRepo := repository.NewPostAnalyticsRepository(db)
	socialAccountRepo := repository.NewSocialAccountRepository(db)
	autoPublishAllowlistRepo := repository.NewAutoPublishAllowlistRepository(db)
	auth := handlers.RequireAuth(sessionRepo, cfg.SessionCookieName)

	// In-process event hub: backend code publishes; the SSE endpoint
	// fans events out to authenticated clients.
	hub := eventhub.New(eventhub.Config{})
	handlers.NewEventsHandler(hub, sessionRepo, auth, 0).Register(app)

	handlers.NewHealthHandler(db, secretStore).Register(app)
	handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
	// CON-97: public self-service signup (POST /api/tenants) + tenant CRU.
	handlers.NewTenantsHandler(db, tenantRepo, userRepo, cfg.SessionCookieName, !cfg.Debug, auth).Register(app)
	// Session cookies are marked Secure in production. Debug mode is the
	// development escape hatch so localhost over plain HTTP still works.
	handlers.NewSessionsHandler(userRepo, sessionRepo, cfg.SessionCookieName, !cfg.Debug).Register(app)
	handlers.NewSettingsHandler(settingRepo, auth).Register(app)
	handlers.NewSecretsHandler(secretStore, auth).Register(app)
	handlers.NewAutoPublishAllowlistHandler(autoPublishAllowlistRepo, auth).Register(app)

	// Zernio integration. Ping, profile bootstrap, and the sync worker
	// all run in background goroutines so Ogen boot never blocks on
	// Zernio reachability. The shutdown hook waits up to 2s for the
	// worker to exit cleanly.
	zernioRT := initZernio(ctx, cfg, secretStore, settingRepo, socialAccountRepo, hub)
	if zernioRT.Bootstrapper != nil {
		handlers.NewZernioHandler(
			zernioRT.Integration,
			zernioRT.Bootstrapper,
			zernioRT.Settings,
			platformRepo,
			socialAccountRepo,
			zernioRT.Worker,
			zernioRT.RateLimiter,
			auth,
		).Register(app)
		app.Hooks().OnShutdown(func() error {
			zernioRT.shutdown()
			return nil
		})
	}

	// Build the publisher list the platforms handler will surface in
	// its enriched response. Empty when no integration is configured —
	// the handler emits `"publishers": []` per platform in that case.
	var pubs []publishers.Publisher
	if zernioRT.Integration != nil && zernioRT.Settings != nil {
		pubs = append(pubs, pubzernio.NewPublisher(
			zernioRT.Integration,
			socialAccountRepo,
			zernioRT.Settings,
		))
	}

	store, err := storage.New(cfg)
	if err != nil {
		return nil, err
	}

	// CON-87 WS3: River background-job queue. Runs on the same
	// database/sql pool as bun (db.DB), so a submit enqueue can join the
	// schedule transaction (CON-78 §9). The worker pool starts below and
	// is drained on shutdown via the Fiber hook.
	//
	// ProfileID resolves lazily so this wiring runs before the
	// bootstrapper has finished, and so a profile id added later via the
	// secrets API is picked up on the next dispatch.
	zernioDeps := queues.ZernioDeps{
		PostRepo:           postRepo,
		PostLogRepo:        postLogRepo,
		PostAttachmentRepo: postAttachmentRepo,
		SocialAccountRepo:  socialAccountRepo,
		SettingRepo:        settingRepo,
		AnalyticsRepo:      postAnalyticsRepo,
		Client:             zernioRT.Integration.Client,
		ProfileID: func(ctx context.Context) (string, error) {
			id, _, err := zernioRT.Settings.Get(ctx, pubzernio.SettingProfileID)
			return id, err
		},
	}

	// The six workers self-register from their init()s; RegisterAll wires them
	// to the River registry with one dependency bundle. The analytics periodic
	// job is only scheduled when the Zernio integration is configured —
	// otherwise every tick would no-op against a disabled client.
	cleanupEvery := time.Hour
	reconcileEvery := 5 * time.Minute
	workers := river.NewWorkers()
	queues.RegisterAll(workers, queues.Deps{
		Zernio:              zernioDeps,
		PostLogRetention:    time.Duration(cfg.PostLogRetentionDays) * 24 * time.Hour,
		ReconcileGrace:      cfg.ReconcileGrace,
		AnalyticsSettings:   zernioRT.Settings,
		AnalyticsHub:        hub,
		AnalyticsWindowDays: cfg.ZernioAnalyticsWindowDays,
	})

	if err := jobs.MigrateRiver(ctx, db.DB); err != nil {
		return nil, err
	}
	riverClient, err := river.NewClient[*sql.Tx](riverdatabasesql.New(db.DB), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.JobWorkers}},
		Workers: workers,
		PeriodicJobs: queues.PeriodicConfig{
			CleanupEvery:     cleanupEvery,
			ReconcileEvery:   reconcileEvery,
			AnalyticsEvery:   cfg.ZernioAnalyticsRefreshInterval,
			IncludeAnalytics: zernioRT.Bootstrapper != nil,
		}.PeriodicJobs(),
	})
	if err != nil {
		return nil, err
	}
	enqueuer := &queues.Enqueuer{Client: riverClient}

	// Expose expvar counters for ops health dashboards (CON-69 §13).
	// Gated by the same auth as the rest of the app so internal
	// counters aren't anonymous. (A River monitoring UI is a follow-up;
	// the old /admin/backlite mount is removed with backlite.)
	app.Get("/debug/vars", auth, adaptor.HTTPHandler(expvar.Handler()))

	if err := riverClient.Start(ctx); err != nil {
		return nil, err
	}
	app.Hooks().OnShutdown(func() error {
		sctx, cancel := context.WithTimeout(context.Background(), cfg.JobShutdownTimeout)
		defer cancel()
		_ = riverClient.Stop(sctx)
		return nil
	})

	// Embedding genkit instance: built once at boot with just the
	// llama plugin and never rebuilt. The Anthropic flows live on a
	// separate, rebuildable instance owned by genkitRuntime — that
	// split keeps an Anthropic key rotation from disturbing the
	// embedder bound here. The dev UI sees only the embedding instance
	// today; that's an acceptable trade-off for hot-reload.
	log.Printf("genkit: initialising (GENKIT_ENV=%s)", os.Getenv("GENKIT_ENV"))

	llamaPlugin, err := embedding.WaitAndNewPlugin(ctx, cfg.EmbedServerURL,
		embedding.DefaultMaxRetries, embedding.DefaultRetryInterval)
	if err != nil {
		return nil, err
	}

	var embedCallbacks embedding.Callbacks
	var embedder ai.Embedder
	if llamaPlugin != nil {
		embeddingG := genkit.Init(ctx, genkit.WithPlugins(llamaPlugin))
		embedCallbacks, embedder, err = embedding.RegisterFlows(embeddingG, llamaPlugin, chunksRepo, pieceRepo, assetFileRepo, store)
		if err != nil {
			return nil, err
		}
	}
	handlers.NewAssetsHandler(pieceRepo, assetFileRepo, store, auth, embedCallbacks.OnMarkdownSave, embedCallbacks.OnPDFProcess).Register(app)

	// Anthropic-backed flows live in a hot-reloadable runtime. boot
	// is allowed to start without an Anthropic key (callbacks return
	// 503 via the handler's IsAnthropicAvailable check); a PUT to
	// /api/secrets/anthropic_api_key triggers a rebuild on the next
	// call.
	// CON-59: one clone service, shared by the REST endpoint and the
	// assistant's clonePost tool. Deep-copies attachments in object
	// storage so clone and source have independent blob lifecycles.
	cloneSvc := clone.New(db, postRepo, postVersionRepo, postAttachmentRepo, platformRepo, postLogRepo, store, hub)
	// CON-68: one restore service, shared by the REST endpoint and the
	// assistant's restoreVersion tool. Non-destructive append-only roll-back.
	restoreSvc := restore.New(db, postRepo, postVersionRepo, postLogRepo, hub)
	// CON-78: one schedule service, shared by POST /:id/schedule, the
	// assistant's schedulePost tool, and the PUT scheduling branch. Owns
	// allowlist routing + transactional persist + Zernio submit enqueue.
	scheduleSvc := schedule.New(db, postRepo, platformRepo, postAttachmentRepo, autoPublishAllowlistRepo, postLogRepo, enqueuer, hub)

	gkRuntime, err := newGenkitRuntime(ctx, genkitDeps{
		cfg:      cfg,
		hub:      hub,
		embedder: embedder,
		contentPlanRepos: content_plan.ContentPlanRepos{
			Campaigns: campaignRepo,
			Assets:    pieceRepo,
			Chunks:    chunksRepo,
			Platforms: platformRepo,
			Posts:     postRepo,
		},
		postAssistRepos: post_assistant.PostAssistantRepos{
			Posts:       postRepo,
			Assets:      pieceRepo,
			Chunks:      chunksRepo,
			Campaigns:   campaignRepo,
			Versions:    postVersionRepo,
			Messages:    postMessageRepo,
			Platforms:   platformRepo,
			Settings:    settingRepo,
			Allowlist:   autoPublishAllowlistRepo,
			Attachments: postAttachmentRepo,
		},
		postQualityRepos: post_quality.PostQualityRepos{
			Posts:       postRepo,
			Campaigns:   campaignRepo,
			Assets:      pieceRepo,
			Chunks:      chunksRepo,
			Platforms:   platformRepo,
			Evaluations: postEvaluationRepo,
			PostLogs:    postLogRepo,
		},
		enrichBriefRepos: enrich_brief.EnrichBriefRepos{
			Campaigns:     campaignRepo,
			CampaignTypes: campaignTypeRepo,
		},
		cloneSvc:    cloneSvc,
		restoreSvc:  restoreSvc,
		scheduleSvc: scheduleSvc,
	}, secretStore)
	if err != nil {
		return nil, err
	}
	log.Println("genkit: all flows registered")

	handlers.NewCampaignTypesHandler(campaignTypeRepo, auth).Register(app)
	handlers.NewCampaignsHandler(campaignRepo, campaignTypeRepo, auth, gkRuntime.GenerateDraft, gkRuntime.IsAnthropicAvailable, gkRuntime.EnrichBrief).Register(app)
	handlers.NewPlatformsHandler(platformRepo, pubs, autoPublishAllowlistRepo, auth).Register(app)
	handlers.NewTagsHandler(tagRepo, auth).Register(app)
	postsHandler := handlers.NewPostsHandler(postRepo, postVersionRepo, postMessageRepo, platformRepo, postAttachmentRepo, auth, gkRuntime.RunPostAssistant, gkRuntime.IsAnthropicAvailable)
	// CON-69 §11: every transition (success/blocked) and validation
	// outcome lands in the Post Log.
	postsHandler.SetPostLogRepo(postLogRepo)
	// CON-69 §5: ReadyForPublish→Scheduled consults the auto-publish
	// allowlist and (for allowlisted platforms) enqueues a submit task
	// transactionally with the status change + log write.
	postsHandler.SetSchedulingDeps(autoPublishAllowlistRepo, enqueuer, db)
	// CON-59: same clone service the assistant uses, behind the REST
	// endpoint that backs the (future) Duplicate button.
	postsHandler.SetCloneService(cloneSvc)
	// CON-68: same restore service the assistant uses, behind the REST
	// endpoint POST /api/posts/:id/restore.
	postsHandler.SetRestoreService(restoreSvc)
	// CON-78: same schedule service the assistant uses, behind the REST
	// endpoint POST /api/posts/:id/schedule and the PUT scheduling branch.
	postsHandler.SetScheduleService(scheduleSvc)
	// CON-85: Post quality assessment agent behind POST /api/posts/:id/assess.
	postsHandler.SetQualityAssessor(gkRuntime.AssessPostQuality)
	// CON-92: cached read behind GET /api/posts/:id/assessment, so the
	// frontend can render an existing evaluation without re-running the model.
	postsHandler.SetEvaluationRepo(postEvaluationRepo)
	// CON-93 FR4: per-post analytics snapshot behind GET /api/posts/:id/analytics,
	// served from the DB (never live-calls the publisher).
	postsHandler.SetAnalyticsRepo(postAnalyticsRepo)
	// Cascade post-attachment S3 cleanup on post delete (CON-73 §2.7).
	// FK CASCADE handles the DB rows; this hook handles the bucket.
	postsHandler.SetOnBeforeDelete(func(ctx context.Context, postID string) error {
		if store == nil {
			return nil
		}
		keys, err := postAttachmentRepo.ListS3KeysByPostID(ctx, postID)
		if err != nil {
			return err
		}
		for _, k := range keys {
			if k == "" {
				continue
			}
			if err := store.Delete(ctx, k); err != nil {
				return err
			}
		}
		return nil
	})
	postsHandler.Register(app)
	handlers.NewPostLogsHandler(postLogRepo, postRepo, auth).Register(app)
	// CON-93 FR5: cross-post analytics overview under its own /api/analytics
	// group (avoids the /api/posts/:id route collision), served from the DB.
	handlers.NewAnalyticsHandler(postAnalyticsRepo, auth).Register(app)

	handlers.NewImagesHandler(store, auth).Register(app)
	handlers.NewPostAttachmentsHandler(postAttachmentRepo, postRepo, store, auth).Register(app)

	// The React SPA is deployed separately (CON-98) — the API serves only
	// /api/* (plus SSE). Non-API routes fall through to a 404.
	return app, nil
}

func defaultErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := errors.AsType[*fiber.Error](err); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}
