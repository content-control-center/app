package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Addr              string `envconfig:"ADDR"                default:":9001"`
	DSN               string `envconfig:"DATABASE_DSN"        default:"file:data/app.db?cache=shared&_pragma=journal_mode(WAL)"`
	Debug             bool   `envconfig:"DEBUG"               default:"false"`
	SessionCookieName string `envconfig:"SESSION_COOKIE_NAME" default:"c3_session"`
	EmbedServerURL    string `envconfig:"EMBED_SERVER_URL"    default:"http://localhost:8080"`

	// Anthropic config.
	AnthropicAPIKey  string `envconfig:"ANTHROPIC_API_KEY"     default:""`
	ModelID          string `envconfig:"MODEL_ID"              default:"claude-sonnet-4-5-20250929"` // claude-haiku-4-5-20251001 for testing
	MaxContextAssets int    `envconfig:"MAX_ASSET_CONTEXT"     default:"15"`
	MaxContextChars  int    `envconfig:"MAX_CONTEXT_CHARS"     default:"10000"`

	// 64K matches Claude 4.x Haiku/Sonnet's max output. Anthropic charges
	// only for tokens actually emitted, so a generous cap costs nothing on
	// short responses but prevents truncation on long rewrites (assistant
	// flow with explanation + full post content + tool inputs combined).
	MaxOutputTokens int64 `envconfig:"MAX_OUTPUT_TOKENS"     default:"64000"`

	// Object storage (S3-compatible: Cloudflare R2, DigitalOcean Spaces, AWS S3).
	// Leave StorageEndpoint empty to disable image uploads.
	StorageEndpoint  string `envconfig:"STORAGE_ENDPOINT"   default:""`
	StorageRegion    string `envconfig:"STORAGE_REGION"     default:"auto"`
	StorageAccessKey string `envconfig:"STORAGE_ACCESS_KEY" default:""`
	StorageSecretKey string `envconfig:"STORAGE_SECRET_KEY" default:""`
	StorageBucket    string `envconfig:"STORAGE_BUCKET"     default:""`
	StoragePublicURL string `envconfig:"STORAGE_PUBLIC_URL" default:""` // CDN/public base URL for returned object URLs

	// Zernio integration. Empty ZernioAPIKey disables the
	// integration entirely; everything else stays defaulted.
	ZernioAPIKey           string        `envconfig:"ZERNIO_API_KEY"            default:""`
	ZernioBaseURL          string        `envconfig:"ZERNIO_BASE_URL"           default:"https://api.zernio.com"`
	ZernioHTTPTimeout      time.Duration `envconfig:"ZERNIO_HTTP_TIMEOUT"       default:"15s"`
	ZernioSyncInterval     time.Duration `envconfig:"ZERNIO_SYNC_INTERVAL"      default:"30s"`
	ZernioSyncIntervalFast time.Duration `envconfig:"ZERNIO_SYNC_INTERVAL_FAST" default:"5s"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
