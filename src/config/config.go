package config

import (
	"time"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Addr  string `envconfig:"ADDR"                default:":9001"`
	DSN   string `envconfig:"DATABASE_DSN"        default:"postgres://ogen:ogen@localhost:5432/ogen?sslmode=disable"`
	Debug bool   `envconfig:"DEBUG"               default:"false"`

	// Connection-pool sizing. Postgres lifts SQLite's single-writer
	// ceiling, so the API runs a real pool shared by bun and the River
	// job queue. Size MaxOpen for combined HTTP + worker load.
	DBMaxOpenConns    int    `envconfig:"DB_MAX_OPEN_CONNS" default:"25"`
	DBMaxIdleConns    int    `envconfig:"DB_MAX_IDLE_CONNS" default:"5"`
	SessionCookieName string `envconfig:"SESSION_COOKIE_NAME" default:"c3_session"`

	// Embeddings (CON-101). Generated via the hosted Gemini Embedding 2 API
	// (google.golang.org/genai through the Genkit googlegenai plugin), replacing
	// the former self-hosted llama-embedserver sidecar. Empty GeminiAPIKey
	// disables embedding entirely (asset saves succeed, no vectors are written,
	// semantic search returns nothing). EmbedDimensions must match the
	// assets_chunks.embedding halfvec(N) column — 3072 is Gemini's native size
	// and is L2-normalized, so cosine search works without renormalization.
	GeminiAPIKey    string `envconfig:"GEMINI_API_KEY"    default:""`
	EmbedModel      string `envconfig:"EMBED_MODEL"       default:"gemini-embedding-2"`
	EmbedDimensions int    `envconfig:"EMBED_DIMENSIONS"  default:"3072"`

	// CORS allowlist for the decoupled UI (CON-98). Comma-separated explicit
	// origins, e.g. "https://app.getogen.com". Empty disables the CORS
	// middleware entirely (same-origin dev, or a UI that reverse-proxies
	// /api). Must never be "*" while credentials are sent — the cookie-bearing
	// UI requires AllowCredentials, which browsers reject alongside a wildcard.
	CORSAllowedOrigins string `envconfig:"CORS_ALLOWED_ORIGINS" default:""`

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

	// PDF parsing microservice (CON-103). The API streams PDF bytes to
	// pdf-service over gRPC — exclusively over the Railway private network — and
	// gets back page-attributed chunks + a thumbnail. Empty PDFServiceAddr
	// disables PDF ingestion (uploads accepted, parsing skipped), mirroring the
	// empty-key pattern above. In prod this is the private hostname
	// (pdf-service.railway.internal:50051); compose/tests use pdf-service:50051.
	// MaxRecvBytes raises the gRPC client receive cap (the response carries chunk
	// text + thumbnail, which can exceed gRPC's 4MB default) — 64 MiB here.
	PDFServiceAddr         string        `envconfig:"PDF_SERVICE_ADDR"           default:""`
	PDFServiceTimeout      time.Duration `envconfig:"PDF_SERVICE_TIMEOUT"        default:"2m"`
	PDFServiceMaxRecvBytes int           `envconfig:"PDF_SERVICE_MAX_RECV_BYTES" default:"67108864"`

	// Zernio integration. Empty ZernioAPIKey disables the
	// integration entirely; everything else stays defaulted.
	ZernioAPIKey           string        `envconfig:"ZERNIO_API_KEY"            default:""`
	ZernioBaseURL          string        `envconfig:"ZERNIO_BASE_URL"           default:"https://zernio.com/api/v1"`
	ZernioHTTPTimeout      time.Duration `envconfig:"ZERNIO_HTTP_TIMEOUT"       default:"15s"`
	ZernioSyncInterval     time.Duration `envconfig:"ZERNIO_SYNC_INTERVAL"      default:"30s"`
	ZernioSyncIntervalFast time.Duration `envconfig:"ZERNIO_SYNC_INTERVAL_FAST" default:"5s"`

	// ZernioEnv namespaces each tenant's Zernio profile name,
	// "Ogen-<ZernioEnv>-<tenant_id>" (CON-102), so profiles created by dev,
	// staging, and prod Ogen instances against the same shared Zernio account
	// stay distinguishable on the Zernio dashboard. Defaults to "dev".
	ZernioEnv string `envconfig:"ZERNIO_ENV" default:"dev"`

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

	// River background-job queue (CON-69 §1, §3; CON-87 WS3). Workers
	// process `submit_post_to_zernio`, `poll_zernio_status`,
	// `cancel_zernio_job`, plus the periodic `reconcile_scheduled_posts`,
	// `cleanup_post_logs`, and `refresh_zernio_analytics`. JobWorkers sizes
	// the worker pool on the default queue; River owns leasing, retry/
	// backoff, and completed-job retention internally.
	JobWorkers int `envconfig:"JOB_WORKERS" default:"4"`
	// Graceful-shutdown wait for in-flight jobs to finish.
	JobShutdownTimeout time.Duration `envconfig:"JOB_SHUTDOWN_TIMEOUT" default:"30s"`

	// Reconciliation grace window (CON-69 §8). A Scheduled post whose
	// scheduled_at + this window has passed without a terminal Zernio
	// status is forced to Failed with a reason that distinguishes
	// reconciliation_timeout from a Zernio-reported failure.
	ReconcileGrace time.Duration `envconfig:"RECONCILE_GRACE" default:"1h"`

	// PostLog retention (CON-69 §11). Older entries are removed by the
	// cleanup_post_logs recurring task. 0 disables cleanup entirely.
	PostLogRetentionDays int `envconfig:"POSTLOG_RETENTION_DAYS" default:"90"`

	// Usage metering & per-tenant cost limits (CON-86). AnalyticsDSN points at
	// the isolated analytics (TimescaleDB) database; empty disables usage
	// recording + enforcement entirely (calls proceed, nothing recorded) and
	// is the graceful-disable default, mirroring empty GeminiAPIKey.
	AnalyticsDSN string `envconfig:"ANALYTICS_DSN" default:""`
	// UsageRetentionDays mirrors PostLogRetentionDays; the analytics-DB
	// retention policy drops usage_events chunks older than this. (The current
	// migration installs a fixed 90-day policy; operators adjust it out of band.)
	UsageRetentionDays int `envconfig:"USAGE_RETENTION_DAYS" default:"90"`
	// Global default spend caps in USD-micros, applied to any tenant without a
	// tenant_usage_limits row; 0 = no default cap for that period. Mode is
	// enforce (block once over) or warn (record + count, proceed).
	UsageDefaultDailyCapMicros   int64  `envconfig:"USAGE_DEFAULT_DAILY_CAP_MICROS"   default:"0"`
	UsageDefaultMonthlyCapMicros int64  `envconfig:"USAGE_DEFAULT_MONTHLY_CAP_MICROS" default:"0"`
	UsageDefaultMode             string `envconfig:"USAGE_DEFAULT_MODE"               default:"enforce"`
	// Optional JSON override of the in-code per-model price map (mirrors
	// QualityWeightProfiles); empty uses the built-in vendor defaults. (Parsing
	// into the registry is a follow-up; the in-code prices are authoritative.)
	UsageModelPrices string `envconfig:"USAGE_MODEL_PRICES" default:""`

	// UsageAdminToken gates PUT /api/usage/limits — raising a cap or disabling
	// enforcement is an operator action, not tenant self-service, and the user
	// model has no admin/owner role yet (CON-86 §15). A caller must present
	// this value in the X-Admin-Token header. EMPTY (the default) FAILS CLOSED:
	// the write route rejects everyone, so tenants cannot lift their own caps.
	UsageAdminToken string `envconfig:"USAGE_ADMIN_TOKEN" default:""`
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
