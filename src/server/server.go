package server

import (
	"context"
	"errors"
	"expvar"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/mikestefanello/backlite"
	backliteui "github.com/mikestefanello/backlite/ui"
	"github.com/uptrace/bun"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/embedding"
	"github.com/content-control-center/app/src/eventhub"
	"github.com/content-control-center/app/src/genkit/flows/content_plan"
	"github.com/content-control-center/app/src/genkit/flows/post_assistant"
	"github.com/content-control-center/app/src/handlers"
	"github.com/content-control-center/app/src/jobs"
	"github.com/content-control-center/app/src/jobs/queues"
	"github.com/content-control-center/app/src/publishers"
	pubzernio "github.com/content-control-center/app/src/publishers/zernio"
	"github.com/content-control-center/app/src/repository"
	"github.com/content-control-center/app/src/secrets"
	"github.com/content-control-center/app/src/storage"
)

func New(ctx context.Context, db *bun.DB, staticFS fs.FS, cfg *config.Config, secretStore secrets.Store) (*fiber.App, error) {
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

	// API routes
	userRepo := repository.NewUserRepository(db)
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
	socialAccountRepo := repository.NewSocialAccountRepository(db)
	autoPublishAllowlistRepo := repository.NewAutoPublishAllowlistRepository(db)
	auth := handlers.RequireAuth(sessionRepo, cfg.SessionCookieName)

	// In-process event hub: backend code publishes; the SSE endpoint
	// fans events out to authenticated clients.
	hub := eventhub.New(eventhub.Config{})
	handlers.NewEventsHandler(hub, sessionRepo, auth, 0).Register(app)

	handlers.NewHealthHandler(db, secretStore).Register(app)
	handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
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

	// CON-69 §1+§3: Backlite background-job queue. Shares the SQLite
	// file with the rest of the app (db.DB exposes the underlying
	// *sql.DB). Schema install is idempotent. The worker pool starts
	// further below and is drained on shutdown via the Fiber hook.
	jobsRT, err := jobs.New(jobs.Config{
		DB:              db.DB,
		Workers:         cfg.BackliteWorkers,
		ReleaseAfter:    cfg.BackliteReleaseAfter,
		CleanupInterval: cfg.BackliteCleanupInterval,
		ShutdownTimeout: cfg.BackliteShutdownTimeout,
	})
	if err != nil {
		return nil, err
	}
	// Recurring Post Log retention sweeper.
	cleanupEvery := time.Hour
	jobsRT.Register(backlite.NewQueue[queues.CleanupPostLogsTask]((&queues.CleanupPostLogsProcessor{
		Repo:      postLogRepo,
		Retention: time.Duration(cfg.PostLogRetentionDays) * 24 * time.Hour,
		Every:     cleanupEvery,
	}).Process))

	// CON-69 §6+§7: Zernio submit + poll queues. Both share the same
	// dependency bundle. ProfileID resolves lazily so this wiring runs
	// before the bootstrapper has finished, and so a profile id added
	// later via the secrets API is picked up on the next dispatch.
	zernioDeps := queues.ZernioDeps{
		PostRepo:           postRepo,
		PostLogRepo:        postLogRepo,
		PostAttachmentRepo: postAttachmentRepo,
		SocialAccountRepo:  socialAccountRepo,
		Client:             zernioRT.Integration.Client,
		ProfileID: func(ctx context.Context) (string, error) {
			id, _, err := zernioRT.Settings.Get(ctx, pubzernio.SettingProfileID)
			return id, err
		},
	}
	jobsRT.Register(backlite.NewQueue[queues.SubmitPostTask]((&queues.SubmitPostProcessor{Deps: zernioDeps}).Process))
	jobsRT.Register(backlite.NewQueue[queues.PollZernioStatusTask]((&queues.PollZernioStatusProcessor{Deps: zernioDeps}).Process))
	jobsRT.Register(backlite.NewQueue[queues.CancelZernioJobTask]((&queues.CancelZernioJobProcessor{Deps: zernioDeps}).Process))

	// CON-69 §8: reconciliation sweeper. Runs every 5 min by default;
	// posts in Scheduled whose scheduled_at + cfg.ReconcileGrace has
	// passed without a terminal status get forced to Failed with a
	// distinct reconciliation_timeout reason.
	reconcileEvery := 5 * time.Minute
	jobsRT.Register(backlite.NewQueue[queues.ReconcileScheduledPostsTask]((&queues.ReconcileScheduledPostsProcessor{
		DB:      db,
		Repo:    postRepo,
		LogRepo: postLogRepo,
		Grace:   cfg.ReconcileGrace,
		Every:   reconcileEvery,
	}).Process))
	if _, err := jobsRT.Client().Add(queues.ReconcileScheduledPostsTask{}).Wait(reconcileEvery).Save(); err != nil {
		log.Printf("jobs: failed to seed reconcile_scheduled_posts: %v", err)
	}
	// Mount the Backlite monitoring UI under the standard auth gate.
	// The handler is an http.ServeMux internally; we adapt it to Fiber
	// and register a wildcard route at the configured base path.
	{
		uiHandler, err := backliteui.NewHandler(backliteui.Config{
			DB:           db.DB,
			BasePath:     "/admin/backlite",
			ReleaseAfter: cfg.BackliteReleaseAfter,
		})
		if err != nil {
			return nil, err
		}
		mux := http.NewServeMux()
		uiHandler.Register(mux)
		app.All("/admin/backlite/*", auth, adaptor.HTTPHandler(mux))
		app.Get("/admin/backlite", auth, adaptor.HTTPHandler(mux))
	}
	// Expose expvar counters for ops health dashboards (CON-69 §13).
	// Gated by the same auth as the rest of the app so internal
	// counters aren't anonymous.
	app.Get("/debug/vars", auth, adaptor.HTTPHandler(expvar.Handler()))
	// Seed the recurring cleanup chain. Subsequent ticks are scheduled
	// from inside Process via backlite.FromContext(ctx).
	if _, addErr := jobsRT.Client().Add(queues.CleanupPostLogsTask{}).Wait(cleanupEvery).Save(); addErr != nil {
		log.Printf("jobs: failed to seed cleanup_post_logs: %v", addErr)
	}
	jobsRT.Start(ctx)
	app.Hooks().OnShutdown(func() error {
		jobsRT.Shutdown(context.Background())
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
			Posts:     postRepo,
			Assets:    pieceRepo,
			Chunks:    chunksRepo,
			Campaigns: campaignRepo,
			Versions:  postVersionRepo,
			Messages:  postMessageRepo,
		},
	}, secretStore)
	if err != nil {
		return nil, err
	}
	log.Println("genkit: all flows registered")

	handlers.NewCampaignTypesHandler(campaignTypeRepo, auth).Register(app)
	handlers.NewCampaignsHandler(campaignRepo, campaignTypeRepo, auth, gkRuntime.GenerateDraft, gkRuntime.IsAnthropicAvailable).Register(app)
	handlers.NewPlatformsHandler(platformRepo, pubs, autoPublishAllowlistRepo, auth).Register(app)
	handlers.NewTagsHandler(tagRepo, auth).Register(app)
	postsHandler := handlers.NewPostsHandler(postRepo, postVersionRepo, postMessageRepo, auth, gkRuntime.RunPostAssistant, gkRuntime.IsAnthropicAvailable)
	// CON-69 §4: Draft→ReadyForPublish runs platform validation against
	// the post's attachments. The handler is a no-op until this is set,
	// keeping handler-test fixtures unaffected.
	postsHandler.SetAttachmentRepo(postAttachmentRepo)
	// CON-69 §11: every transition (success/blocked) and validation
	// outcome lands in the Post Log.
	postsHandler.SetPostLogRepo(postLogRepo)
	// CON-69 §5: ReadyForPublish→Scheduled consults the auto-publish
	// allowlist and (for allowlisted platforms) enqueues a submit task
	// transactionally with the status change + log write.
	postsHandler.SetSchedulingDeps(autoPublishAllowlistRepo, jobsRT.Client(), db)
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

	handlers.NewImagesHandler(store, auth).Register(app)
	handlers.NewPostAttachmentsHandler(postAttachmentRepo, postRepo, store, auth).Register(app)

	// Serve the embedded React SPA for all non-API routes.
	app.Use("/", filesystem.New(filesystem.Config{
		Root:         http.FS(staticFS),
		Index:        "index.html",
		NotFoundFile: "index.html", // enables client-side routing
		Browse:       false,
	}))

	return app, nil
}

func defaultErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := errors.AsType[*fiber.Error](err); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}
