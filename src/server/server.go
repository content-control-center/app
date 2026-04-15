package server

import (
	"context"
	"errors"
	"io/fs"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/uptrace/bun"

	"github.com/content-control-center/app/src/config"
	"github.com/content-control-center/app/src/genkit/flows/content_plan"
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
	})

	app.Use(recover.New())
	app.Use(logger.New())

	// API routes
	userRepo := repository.NewUserRepository(db)
	sessionRepo := repository.NewSessionRepository(db)
	settingRepo := repository.NewSettingRepository(db)
	tagRepo := repository.NewTagRepository(db)
	pieceRepo := repository.NewAssetRepository(db, tagRepo)
	embeddingRepo := repository.NewAssetsEmbeddingRepository(db)
	platformRepo := repository.NewPlatformRepository(db)
	campaignTypeRepo := repository.NewCampaignTypeRepository(db)
	campaignRepo := repository.NewCampaignRepository(db, tagRepo, platformRepo, campaignTypeRepo)
	postRepo := repository.NewPostRepository(db)
	auth := handlers.RequireAuth(sessionRepo, cfg.SessionCookieName)

	handlers.NewHealthHandler(db).Register(app)
	handlers.NewUsersHandler(userRepo, settingRepo, auth).Register(app)
	// Session cookies are marked Secure in production. Debug mode is the
	// development escape hatch so localhost over plain HTTP still works.
	handlers.NewSessionsHandler(userRepo, sessionRepo, cfg.SessionCookieName, !cfg.Debug).Register(app)
	handlers.NewSettingsHandler(settingRepo, auth).Register(app)

	onSave, embedder, err := initEmbedding(ctx, cfg, embeddingRepo)
	if err != nil {
		return nil, err
	}
	handlers.NewAssetsHandler(pieceRepo, auth, onSave).Register(app)

	contentPlanRepos := content_plan.ContentPlanRepos{
		Campaigns:  campaignRepo,
		Assets:     pieceRepo,
		Embeddings: embeddingRepo,
		Platforms:  platformRepo,
		Posts:      postRepo,
	}
	generateDraft, err := initContentPlan(ctx, cfg, embedder, contentPlanRepos)
	if err != nil {
		return nil, err
	}
	handlers.NewCampaignTypesHandler(campaignTypeRepo, auth).Register(app)
	handlers.NewCampaignsHandler(campaignRepo, campaignTypeRepo, auth, generateDraft).Register(app)
	handlers.NewPlatformsHandler(platformRepo, auth).Register(app)
	handlers.NewTagsHandler(tagRepo, auth).Register(app)
	handlers.NewPostsHandler(postRepo, auth).Register(app)

	store, err := storage.New(cfg)
	if err != nil {
		return nil, err
	}
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
