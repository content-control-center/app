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

	// Content-plan batching. The flow generates posts in K-sized batches in
	// parallel; the defaults are sized so a 64K-output Sonnet call comfortably
	// returns 30 posts with headroom, and so an account with default tier
	// limits doesn't stall on parallel ITPM bursts. Tune up for plans with
	// 200+ posts where wall time matters.
	MaxPostsPerBatch   int `envconfig:"MAX_POSTS_PER_BATCH"   default:"30"`
	MaxParallelBatches int `envconfig:"MAX_PARALLEL_BATCHES"  default:"5"`

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
	ZernioBaseURL          string        `envconfig:"ZERNIO_BASE_URL"           default:"https://zernio.com/api/v1"`
	ZernioHTTPTimeout      time.Duration `envconfig:"ZERNIO_HTTP_TIMEOUT"       default:"15s"`
	ZernioSyncInterval     time.Duration `envconfig:"ZERNIO_SYNC_INTERVAL"      default:"30s"`
	ZernioSyncIntervalFast time.Duration `envconfig:"ZERNIO_SYNC_INTERVAL_FAST" default:"5s"`

	// Optional post-OAuth redirect target. When set, every connect
	// link Zernio issues will send the user here after authorization
	// succeeds, with ?connected=<platform>&profileId=<id>&accountId=<id>&username=<name>
	// appended by Zernio. Leaving this empty falls back to Zernio's
	// default success page.
	ZernioRedirectURL string `envconfig:"ZERNIO_REDIRECT_URL" default:""`

	// Envelope encryption. KEKPath points at a Docker-volume directory;
	// the actual key file lives at <KEKPath>/kek.v1. The versioned
	// filename leaves room for KEK rotation later without a path
	// migration. Default is a relative ./kek for local dev; the
	// Docker image mounts /var/lib/ogen/keys.
	KEKPath string `envconfig:"OGEN_KEK_PATH" default:"./kek"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
