package config

import "github.com/kelseyhightower/envconfig"

type Config struct {
	Addr              string `envconfig:"ADDR"                default:":9001"`
	DSN               string `envconfig:"DATABASE_DSN"        default:"file:data/app.db?cache=shared&_pragma=journal_mode(WAL)"`
	Debug             bool   `envconfig:"DEBUG"               default:"false"`
	SessionCookieName string `envconfig:"SESSION_COOKIE_NAME" default:"c3_session"`
	EmbedServerURL    string `envconfig:"EMBED_SERVER_URL"    default:"http://localhost:8080"`

	// Anthropic config.
	AnthropicAPIKey string `envconfig:"ANTHROPIC_API_KEY"   default:""`
	ModelID         string `envconfig:"MODEL_ID"            default:"claude-haiku-4-5-20251001"` // claude-sonnet-4-5-20250929
	MaxPieceContext int    `envconfig:"MAX_PIECE_CONTEXT"   default:"15"`

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
