-- CON-125 Track B: post_analytics moves out of the main DB into the isolated
-- analytics database as an APPEND-ONLY, per-refresh time series. Each Zernio
-- analytics refresh writes a new timestamped snapshot per post; reads select the
-- latest snapshot per post. This gives historical trend lines for free and
-- consolidates all analytics/behavioural data in TimescaleDB.
--
-- Named post_analytics_snapshots (not post_analytics) so the same migration set
-- can run on a database that already holds the legacy main-DB post_analytics
-- table (integration tests provision one physical DB and run both migration
-- sets) without a name collision. The legacy table is the backfill source and is
-- dropped by a separate later main-DB migration.
--
-- Post display fields (publisher/platform/title/published_at) are DENORMALISED
-- onto each row: this table lives in a different database from posts/platforms,
-- so the overview read can no longer JOIN to them. They reflect the post at
-- refresh time (point-in-time), which is correct for a snapshot.
--
-- No FOREIGN KEY on tenant_id/post_id: this database is isolated. Tenant
-- isolation is enforced app-side by the TenantScoped hooks.
CREATE TABLE post_analytics_snapshots (
    id                   TEXT        NOT NULL,      -- per-snapshot id
    tenant_id            TEXT        NOT NULL,
    post_id              TEXT        NOT NULL,
    publisher_post_id    TEXT        NOT NULL,
    publisher            TEXT        NOT NULL,      -- denormalised — was posts.publisher
    platform             TEXT,                      -- denormalised platform name — was platforms.name
    title                TEXT,                      -- denormalised — was posts.title
    published_at         TIMESTAMPTZ,               -- denormalised — was posts.published_at
    impressions          BIGINT      NOT NULL DEFAULT 0,
    reach                BIGINT      NOT NULL DEFAULT 0,
    likes                BIGINT      NOT NULL DEFAULT 0,
    comments             BIGINT      NOT NULL DEFAULT 0,
    shares               BIGINT      NOT NULL DEFAULT 0,
    saves                BIGINT      NOT NULL DEFAULT 0,
    clicks               BIGINT      NOT NULL DEFAULT 0,
    views                BIGINT      NOT NULL DEFAULT 0,
    engagement_rate      DOUBLE PRECISION NOT NULL DEFAULT 0,
    platform_analytics   jsonb,                     -- per-platform breakdown (unchanged shape)
    sync_status          TEXT,
    metrics_last_updated TIMESTAMPTZ,               -- publisher's compute time (staleness signal)
    raw_json             TEXT,
    occurred_at          TIMESTAMPTZ NOT NULL DEFAULT now()  -- the refresh time (was last_refreshed_at)
);

-- Latest-per-post reads hit (post_id, occurred_at DESC); the tenant index serves
-- the overview's tenant-scoped scan.
CREATE INDEX idx_post_analytics_snapshots_post_time   ON post_analytics_snapshots (post_id, occurred_at DESC);
CREATE INDEX idx_post_analytics_snapshots_tenant_time ON post_analytics_snapshots (tenant_id, occurred_at DESC);

-- TimescaleDB upgrade — applied only where the extension is available, so the
-- statements above stand alone on vanilla Postgres (mirrors usage_events /
-- activity_events).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
        CREATE EXTENSION IF NOT EXISTS timescaledb;

        PERFORM create_hypertable('post_analytics_snapshots', 'occurred_at',
            chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);

        ALTER TABLE post_analytics_snapshots SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'tenant_id'
        );
        PERFORM add_compression_policy('post_analytics_snapshots', INTERVAL '7 days', if_not_exists => TRUE);

        -- 90-day retention mirrors the analytics window (CON-93 §6 / CON-125).
        PERFORM add_retention_policy('post_analytics_snapshots', INTERVAL '90 days', if_not_exists => TRUE);
    END IF;
END $$;
