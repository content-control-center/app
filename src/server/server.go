package server

import (
	"context"
	"database/sql"
	"errors"
	"expvar"
	"log/slog"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/uptrace/bun"

	"github.com/ogen-app/ogen/src/campaign_actions/overview"
	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/eventhub"
	"github.com/ogen-app/ogen/src/genkit/flows/campaign_assistant"
	"github.com/ogen-app/ogen/src/genkit/flows/content_plan"
	"github.com/ogen-app/ogen/src/genkit/flows/enrich_brief"
	"github.com/ogen-app/ogen/src/genkit/flows/post_assistant"
	"github.com/ogen-app/ogen/src/genkit/flows/post_quality"
	"github.com/ogen-app/ogen/src/handlers"
	"github.com/ogen-app/ogen/src/jobs"
	"github.com/ogen-app/ogen/src/jobs/queues"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/pdfclient"
	"github.com/ogen-app/ogen/src/post_actions/clone"
	"github.com/ogen-app/ogen/src/post_actions/restore"
	"github.com/ogen-app/ogen/src/post_actions/schedule"
	"github.com/ogen-app/ogen/src/publishers"
	pubzernio "github.com/ogen-app/ogen/src/publishers/zernio"
	"github.com/ogen-app/ogen/src/repository"
	"github.com/ogen-app/ogen/src/secrets"
	"github.com/ogen-app/ogen/src/storage"
	"github.com/ogen-app/ogen/src/usage"
)

// TODO: refactor this function
func New(ctx context.Context, db, analyticsDB *bun.DB, cfg *config.Config, secretStore secrets.Store) (*fiber.App, error) {
	app := fiber.New(fiber.Config{
		ErrorHandler: defaultErrorHandler,
		// WriteTimeout 0 disables the per-response write deadline so that SSE
		// streams (e.g. /generate-draft) are not forcibly closed mid-flight.
		WriteTimeout: 0,
		// Allow batched markdown uploads (up to 10 MB per file).
		BodyLimit: 100 << 20,
	})

	app.Use(recover.New())
	// Per-request correlation id (CON-107): honours an inbound X-Request-ID,
	// otherwise generates one, echoes it on the response, and stores it under
	// logging.RequestIDKey so the slog ContextHandler attaches it to every line
	// made with c.Context().
	app.Use(requestid.New(requestid.Config{ContextKey: logging.RequestIDKey}))
	app.Use(accessLog())

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
	campaignMessageRepo := repository.NewCampaignAssistantMessageRepository(db)
	// CON-113: one overview service, shared by the REST endpoint and the
	// Campaign Assistant's getCampaignOverview tool. Not gated by the Anthropic
	// key — it's a plain tenant-scoped DB read.
	campaignOverviewSvc := overview.New(campaignRepo, postRepo, platformRepo)
	postAttachmentRepo := repository.NewPostAttachmentRepository(db)
	postLogRepo := repository.NewPostLogRepository(db)
	postEvaluationRepo := repository.NewPostEvaluationRepository(db)
	postAnalyticsRepo := repository.NewPostAnalyticsRepository(db)
	socialAccountRepo := repository.NewSocialAccountRepository(db)
	autoPublishAllowlistRepo := repository.NewAutoPublishAllowlistRepository(db)
	auth := handlers.RequireAuth(sessionRepo, cfg.SessionCookieName)

	// CON-86: apply any operator price-map override (USAGE_MODEL_PRICES) before
	// metering starts; a malformed payload or unknown vendor fails boot.
	if err := usage.ApplyModelPrices(cfg.UsageModelPrices); err != nil {
		return nil, err
	}
	// usage metering + per-tenant cost enforcement. recorder/checker are nil
	// when analytics is disabled — both are nil-safe in the flows.
	usageWiring := initUsage(cfg, db, analyticsDB)
	handlers.NewUsageHandler(usageWiring.events, usageWiring.limits, usageWiring.defaults, auth, cfg.UsageAdminToken).Register(app)

	// In-process event hub: backend code publishes; the SSE endpoint
	// fans events out to authenticated clients.
	hub := eventhub.New(eventhub.Config{})
	handlers.NewEventsHandler(hub, sessionRepo, auth, 0).Register(app)

	handlers.NewHealthHandler(db, secretStore).Register(app)
	handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
	// CON-97 signup + CON-102 eager Zernio profile provisioning are registered
	// below, after the River enqueuer is built (signup enqueues a bootstrap job
	// in its transaction).
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
	zernioRT := initZernio(ctx, cfg, secretStore, settingRepo, socialAccountRepo, hub, usageWiring.recorder)
	// Registered unconditionally so /api/integrations/zernio/* (incl. the
	// unauthenticated /health) always exists. When no key is set the endpoints
	// report/return integration_disabled; setting zernio_api_key via the
	// secrets API enables it with no reboot (see initZernio's subscription).
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

	// Build the publisher list the platforms handler will surface in its
	// enriched response. The Zernio publisher is always present (the runtime is
	// always wired); it self-reports connection state and resolves the API key
	// per call, so it works once zernio_api_key is set with no reboot.
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

	// CON-103: gRPC client for the PDF parsing microservice over the Railway
	// private network. nil when PDF_SERVICE_ADDR is unset; closed on shutdown.
	pdfClient, err := pdfclient.New(pdfclient.Config{
		Addr:         cfg.PDFServiceAddr,
		Timeout:      cfg.PDFServiceTimeout,
		MaxRecvBytes: cfg.PDFServiceMaxRecvBytes,
	})
	if err != nil {
		return nil, err
	}
	if pdfClient != nil {
		app.Hooks().OnShutdown(func() error { return pdfClient.Close() })
	}

	// Embedding (Gemini) is initialised here — before the River registry —
	// because the process_pdf worker (CON-103) needs the embedder in its deps.
	// The returned embedder is a stable reloadable wrapper (always non-nil): when
	// gemini_api_key is unset it reports unavailable, so PDF ingestion + semantic
	// search are dormant until a key is added via the secrets API (CON-104), with
	// no restart; markdown/JSON saves still succeed meanwhile.
	slog.Info("genkit initialising", logging.AttrComponent, "genkit", "genkit_env", os.Getenv("GENKIT_ENV"))
	embedCallbacks, embedder, err := initEmbedding(ctx, cfg, chunksRepo, pieceRepo, assetFileRepo, store, secretStore, usageWiring.recorder)
	if err != nil {
		return nil, err
	}

	// PDF ingestion is live when the parser and storage are present; embedder
	// availability is checked per-run by the worker (a key set later re-enables
	// it without a restart), not gated at boot. The Client field is left nil
	// otherwise so the worker no-ops.
	pdfIngestEnabled := pdfClient != nil && store != nil
	pdfDeps := queues.PDFDeps{
		Embedder:   embedder,
		Storage:    store,
		Assets:     pieceRepo,
		Chunks:     chunksRepo,
		Files:      assetFileRepo,
		Recorder:   usageWiring.recorder,
		EmbedModel: cfg.EmbedModel,
	}
	if pdfIngestEnabled {
		pdfDeps.Client = pdfClient
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
		Recorder:           usageWiring.recorder,
		ProfileID: func(ctx context.Context) (string, error) {
			id, _, err := zernioRT.Settings.Get(ctx, pubzernio.SettingProfileID)
			return id, err
		},
	}

	// The six workers self-register from their init()s; RegisterAll wires them
	// to the River registry with one dependency bundle. The analytics periodic
	// job is always scheduled; it is profile-driven, so when Zernio is disabled
	// (no key / no profiles) each tick is a harmless no-op, and it starts
	// producing once a key is set via the secrets API — no reboot.
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
		// CON-102: eager per-tenant profile provisioning at signup.
		ProfileBootstrapper: zernioRT.Bootstrapper,
		Integration:         zernioRT.Integration,
		// CON-103: PDF ingestion worker deps.
		PDF: pdfDeps,
	})

	if err := jobs.MigrateRiver(ctx, db.DB); err != nil {
		return nil, err
	}
	riverClient, err := river.NewClient[*sql.Tx](riverdatabasesql.New(db.DB), &river.Config{
		// Route River's internal logging through the shared structured logger
		// (CON-107) so job-queue lines join the same stream and format.
		Logger:  slog.Default(),
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.JobWorkers}},
		Workers: workers,
		PeriodicJobs: queues.PeriodicConfig{
			CleanupEvery:     cleanupEvery,
			ReconcileEvery:   reconcileEvery,
			AnalyticsEvery:   cfg.ZernioAnalyticsRefreshInterval,
			IncludeAnalytics: true,
		}.PeriodicJobs(),
	})
	if err != nil {
		return nil, err
	}
	enqueuer := &queues.Enqueuer{Client: riverClient}

	// CON-97: public self-service signup (POST /api/tenants) + tenant CRU.
	// CON-102: signup enqueues an eager Zernio profile-bootstrap job in its
	// transaction via the enqueuer, so the registration here waits until the
	// River client exists.
	handlers.NewTenantsHandler(db, tenantRepo, userRepo, enqueuer, cfg.SessionCookieName, !cfg.Debug, auth).Register(app)

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

	// Drain the usage recorder LAST. Fiber runs OnShutdown hooks in registration
	// order, so this must come after the river/zernio producer hooks above:
	// otherwise recorder.loop() can drain and exit while a worker is still
	// calling Record(), silently dropping queued usage events. Nil-safe when
	// analytics is disabled.
	if usageWiring.recorder != nil {
		app.Hooks().OnShutdown(func() error {
			sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			return usageWiring.recorder.Close(sctx)
		})
	}

	// CON-103: PDF ingestion goes through the process_pdf River job — the handler
	// stores original.pdf and enqueues in its transaction. The enqueuer is wired
	// only when ingestion is live; otherwise PDF uploads create a pending asset
	// and skip processing. Markdown/JSON embedding still uses OnMarkdownSave.
	var pdfJobs handlers.PDFIngestEnqueuer
	if pdfIngestEnabled {
		pdfJobs = enqueuer
	}
	handlers.NewAssetsHandler(pieceRepo, assetFileRepo, store, db, pdfJobs, auth, embedCallbacks.OnMarkdownSave).Register(app)

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
		campaignAssistRepos: campaign_assistant.CampaignAssistantRepos{
			Messages:  campaignMessageRepo,
			Campaigns: campaignRepo,
			Posts:     postRepo,
		},
		campaignOverviewSvc: campaignOverviewSvc,
		cloneSvc:            cloneSvc,
		restoreSvc:          restoreSvc,
		scheduleSvc:         scheduleSvc,
		recorder:            usageWiring.recorder,
		checker:             usageWiring.checker,
	}, secretStore)
	if err != nil {
		return nil, err
	}
	slog.Info("genkit flows registered", logging.AttrComponent, "genkit")

	handlers.NewCampaignTypesHandler(campaignTypeRepo, auth).Register(app)
	campaignsHandler := handlers.NewCampaignsHandler(campaignRepo, campaignTypeRepo, auth, gkRuntime.GenerateDraft, gkRuntime.IsAnthropicAvailable, gkRuntime.EnrichBrief, campaignMessageRepo, gkRuntime.RunCampaignAssistant)
	campaignsHandler.SetOverviewService(campaignOverviewSvc)
	campaignsHandler.Register(app)
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
	// CON-103: PDF attachment page-count + thumbnail now come from pdf-service.
	// nil pdfClient (PDF_SERVICE_ADDR unset) degrades gracefully (no page count
	// / thumbnail), so it's wired only when present.
	var attachmentRenderer handlers.PDFRenderer
	if pdfClient != nil {
		attachmentRenderer = pdfClient
	}
	handlers.NewPostAttachmentsHandler(postAttachmentRepo, postRepo, store, attachmentRenderer, auth).Register(app)

	// The React SPA is deployed separately (CON-98) — the API serves only
	// /api/* (plus SSE). Non-API routes fall through to a 404.
	return app, nil
}

func defaultErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := errors.AsType[*fiber.Error](err); ok {
		code = e.Code
	}
	// Server errors were previously swallowed — only a JSON body reached the
	// client, nothing was logged (CON-107). Log 5xx at ERROR with request
	// context; 4xx is a client problem, not a server fault, so it is not logged
	// as an error here (it still appears in the access log).
	if code >= 500 {
		slog.ErrorContext(c.Context(), "request failed",
			logging.AttrComponent, "http",
			"method", c.Method(),
			"path", c.Path(),
			"status", code,
			logging.AttrError, err)
	}
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}

// accessLog emits exactly one structured line per request, replacing Fiber's
// default text logger (CON-107). It runs after the handler so the request,
// tenant, and user ids set by downstream middleware are attached by the slog
// ContextHandler via c.Context(). Like Fiber's own logger middleware it invokes
// the app ErrorHandler when the chain returns an error, so the logged status
// reflects the final response (e.g. 5xx) rather than the pre-handler default.
func accessLog() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		chainErr := c.Next()
		if chainErr != nil {
			if herr := c.App().ErrorHandler(c, chainErr); herr != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}

		status := c.Response().StatusCode()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		}
		slog.Default().LogAttrs(c.Context(), level, "request",
			slog.String(logging.AttrComponent, "http"),
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", status),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
			slog.Int("bytes", len(c.Response().Body())),
			slog.String("ip", c.IP()),
		)
		return nil
	}
}
