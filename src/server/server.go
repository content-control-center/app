package server

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/uptrace/bun"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/anthropic"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/embedding"
	"github.com/content-control-center/app/src/eventhub"
	"github.com/content-control-center/app/src/genkit/flows/content_plan"
	"github.com/content-control-center/app/src/genkit/flows/post_assistant"
	"github.com/content-control-center/app/src/handlers"
	"github.com/content-control-center/app/src/repository"
	"github.com/content-control-center/app/src/storage"
)

func New(ctx context.Context, db *bun.DB, staticFS fs.FS, cfg *config.Config) (*fiber.App, error) {
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
	socialAccountRepo := repository.NewSocialAccountRepository(db)
	auth := handlers.RequireAuth(sessionRepo, cfg.SessionCookieName)

	// In-process event hub: backend code publishes; the SSE endpoint
	// fans events out to authenticated clients.
	hub := eventhub.New(eventhub.Config{})
	handlers.NewEventsHandler(hub, sessionRepo, auth, 0).Register(app)

	handlers.NewHealthHandler(db).Register(app)
	handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
	// Session cookies are marked Secure in production. Debug mode is the
	// development escape hatch so localhost over plain HTTP still works.
	handlers.NewSessionsHandler(userRepo, sessionRepo, cfg.SessionCookieName, !cfg.Debug).Register(app)
	handlers.NewSettingsHandler(settingRepo, auth).Register(app)

	// Zernio integration. Ping, profile bootstrap, and the sync worker
	// all run in background goroutines so Ogen boot never blocks on
	// Zernio reachability. The shutdown hook waits up to 2s for the
	// worker to exit cleanly.
	zernioRT := initZernio(ctx, cfg, settingRepo, socialAccountRepo, hub)
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

	store, err := storage.New(cfg)
	if err != nil {
		return nil, err
	}

	// Single shared Genkit instance with all plugins so every flow is
	// discoverable by the Genkit Dev UI. Plugins are collected first, then
	// passed to a single genkit.Init() call.
	var generateDraft func(context.Context, string, content_plan.OnEventFunc) (*content_plan.ContentPlanResponse, error)
	var assistantCallback func(context.Context, post_assistant.PostAssistantRequest, post_assistant.OnEventFunc) (*post_assistant.PostAssistantResponse, error)

	log.Printf("genkit: initialising (GENKIT_ENV=%s)", os.Getenv("GENKIT_ENV"))

	var plugins []api.Plugin
	if cfg.AnthropicAPIKey != "" {
		plugins = append(plugins, &anthropic.Anthropic{})
	}
	llamaPlugin, err := embedding.WaitAndNewPlugin(ctx, cfg.EmbedServerURL,
		embedding.DefaultMaxRetries, embedding.DefaultRetryInterval)
	if err != nil {
		return nil, err
	}
	if llamaPlugin != nil {
		plugins = append(plugins, llamaPlugin)
	}

	g := genkit.Init(ctx, genkit.WithPlugins(plugins...))

	// Register embedding flows (no-op when llama plugin is nil).
	var callbacks embedding.Callbacks
	var embedder ai.Embedder
	if llamaPlugin != nil {
		callbacks, embedder, err = embedding.RegisterFlows(g, llamaPlugin, chunksRepo, pieceRepo, assetFileRepo, store)
		if err != nil {
			return nil, err
		}
	}
	handlers.NewAssetsHandler(pieceRepo, assetFileRepo, store, auth, callbacks.OnMarkdownSave, callbacks.OnPDFProcess).Register(app)

	if cfg.AnthropicAPIKey != "" {
		contentPlanRepos := content_plan.ContentPlanRepos{
			Campaigns: campaignRepo,
			Assets:    pieceRepo,
			Chunks:    chunksRepo,
			Platforms: platformRepo,
			Posts:     postRepo,
		}
		generateDraft, err = initContentPlan(g, cfg, embedder, hub, contentPlanRepos)
		if err != nil {
			return nil, err
		}

		postAssistantRepos := post_assistant.PostAssistantRepos{
			Posts:     postRepo,
			Assets:    pieceRepo,
			Chunks:    chunksRepo,
			Campaigns: campaignRepo,
			Versions:  postVersionRepo,
			Messages:  postMessageRepo,
		}
		assistantCallback, err = initPostAssistant(g, cfg, embedder, hub, postAssistantRepos)
		if err != nil {
			return nil, err
		}
	}
	log.Println("genkit: all flows registered")

	handlers.NewCampaignTypesHandler(campaignTypeRepo, auth).Register(app)
	handlers.NewCampaignsHandler(campaignRepo, campaignTypeRepo, auth, generateDraft).Register(app)
	handlers.NewPlatformsHandler(platformRepo, auth).Register(app)
	handlers.NewTagsHandler(tagRepo, auth).Register(app)
	handlers.NewPostsHandler(postRepo, postVersionRepo, postMessageRepo, auth, assistantCallback).Register(app)

	handlers.NewImagesHandler(store, auth).Register(app)

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
