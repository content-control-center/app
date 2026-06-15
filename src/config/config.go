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
	// todo: remove from config entirely
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

	// Post quality assessment (CON-85). Scoring runs on Sonnet 4.5 by
	// default — Haiku underdelivered (terse, omitting per-dimension prose) —
	// specified separately from ModelID so the scoring model can be tuned
	// independently. QualityWeightProfiles is an optional JSON override for
	// the per-PlatformPostType weight profiles; empty uses the built-in
	// defaults (post_quality.DefaultWeights). Each profile's four weights
	// must sum to 1.0. Example:
	//   {"profiles":{"reel":{"correctness":0.2,"clarity":0.15,"engagement":0.4,"delivery":0.25}}}
	QualityModelID        string `envconfig:"QUALITY_MODEL_ID"        default:"claude-sonnet-4-5-20250929"`
	QualityWeightProfiles string `envconfig:"QUALITY_WEIGHT_PROFILES" default:""`

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

	// Analytics refresh (CON-93 §6 FR3). The refresh_zernio_analytics
	// queue batch-fetches engagement analytics on this cadence and only
	// considers posts published within the lookback window (Zernio caps
	// the analytics range at 366 days).
	ZernioAnalyticsRefreshInterval time.Duration `envconfig:"ZERNIO_ANALYTICS_REFRESH_INTERVAL" default:"30m"`
	ZernioAnalyticsWindowDays      int           `envconfig:"ZERNIO_ANALYTICS_WINDOW_DAYS"      default:"90"`

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

	// Backlite background-job queue (CON-69 §1, §3). Workers process
	// `submit_post_to_zernio`, `poll_zernio_status`, `cancel_zernio_job`,
	// `reconcile_scheduled_posts`, and `cleanup_post_logs`. ReleaseAfter
	// is the per-task lease window — long enough that the longest
	// expected Zernio call completes before another worker thinks the
	// task was abandoned. Cleanup interval keeps the backlite tables
	// trimmed of completed-task rows past their retention window.
	BackliteWorkers         int           `envconfig:"BACKLITE_WORKERS"          default:"4"`
	BackliteReleaseAfter    time.Duration `envconfig:"BACKLITE_RELEASE_AFTER"    default:"5m"`
	BackliteCleanupInterval time.Duration `envconfig:"BACKLITE_CLEANUP_INTERVAL" default:"1h"`
	// Graceful-shutdown wait for in-flight tasks to finish.
	BackliteShutdownTimeout time.Duration `envconfig:"BACKLITE_SHUTDOWN_TIMEOUT" default:"30s"`

	// Reconciliation grace window (CON-69 §8). A Scheduled post whose
	// scheduled_at + this window has passed without a terminal Zernio
	// status is forced to Failed with a reason that distinguishes
	// reconciliation_timeout from a Zernio-reported failure.
	ReconcileGrace time.Duration `envconfig:"RECONCILE_GRACE" default:"1h"`

	// PostLog retention (CON-69 §11). Older entries are removed by the
	// cleanup_post_logs recurring task. 0 disables cleanup entirely.
	PostLogRetentionDays int `envconfig:"POSTLOG_RETENTION_DAYS" default:"90"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
