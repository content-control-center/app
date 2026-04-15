package config

import "github.com/kelseyhightower/envconfig"

type Config struct {
	Addr              string `envconfig:"ADDR"                default:":9001"`
	DSN               string `envconfig:"DATABASE_DSN"        default:"file:data/app.db?cache=shared&_pragma=journal_mode(WAL)"`
	Debug             bool   `envconfig:"DEBUG"               default:"false"`
	SessionCookieName string `envconfig:"SESSION_COOKIE_NAME" default:"c3_session"`
	EmbedServerURL    string `envconfig:"EMBED_SERVER_URL"    default:"http://localhost:8080"`

	// Anthropic config.
	AnthropicAPIKey string `envconfig:"ANTHROPIC_API_KEY"     default:""`
	ModelID         string `envconfig:"MODEL_ID"              default:"claude-sonnet-4-5-20250929"` // claude-haiku-4-5-20251001 for testing
	MaxAssetContext int    `envconfig:"MAX_PIECE_CONTEXT"     default:"15"`

	// Token math: 30 posts × ~200 tokens/post = ~6,000 tokens. The default of 8,192 comfortably covers that. If you
	// switch to Sonnet (which supports 64K output), set MAX_OUTPUT_TOKENS=32768 in the app environment.
	MaxOutputTokens int64 `envconfig:"MAX_OUTPUT_TOKENS"     default:"32000"`

	// Object storage (S3-compatible: Cloudflare R2, DigitalOcean Spaces, AWS S3).
	// Leave StorageEndpoint empty to disable image uploads.
	StorageEndpoint  string `envconfig:"STORAGE_ENDPOINT"   default:""`
	StorageRegion    string `envconfig:"STORAGE_REGION"     default:"auto"`
	StorageAccessKey string `envconfig:"STORAGE_ACCESS_KEY" default:""`
	StorageSecretKey string `envconfig:"STORAGE_SECRET_KEY" default:""`
	StorageBucket    string `envconfig:"STORAGE_BUCKET"     default:""`
	StoragePublicURL string `envconfig:"STORAGE_PUBLIC_URL" default:""` // CDN/public base URL for returned object URLs
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
