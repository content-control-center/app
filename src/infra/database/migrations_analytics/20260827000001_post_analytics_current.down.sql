-- Reverse of CON-236: fold the current-state table back into a single fat,
-- append-per-row post_analytics_snapshots. Best-effort — the verbatim raw_json
-- is NOT restorable (it was dropped) and the per-platform breakdown survives
-- only for each post's latest state (from post_analytics_current). This mirrors
-- the drop_post_analytics precedent, where historical detail isn't restored.

-- 1. Rebuild the fat table (original 20260728000002 shape) under a temp name.
CREATE TABLE post_analytics_snapshots_fat (
    id                   TEXT        NOT NULL,
    tenant_id            TEXT        NOT NULL,
    post_id              TEXT        NOT NULL,
    publisher_post_id    TEXT        NOT NULL,
    publisher            TEXT        NOT NULL,
    platform             TEXT,
    title                TEXT,
    published_at         TIMESTAMPTZ,
    impressions          BIGINT      NOT NULL DEFAULT 0,
    reach                BIGINT      NOT NULL DEFAULT 0,
    likes                BIGINT      NOT NULL DEFAULT 0,
    comments             BIGINT      NOT NULL DEFAULT 0,
    shares               BIGINT      NOT NULL DEFAULT 0,
    saves                BIGINT      NOT NULL DEFAULT 0,
    clicks               BIGINT      NOT NULL DEFAULT 0,
    views                BIGINT      NOT NULL DEFAULT 0,
    engagement_rate      DOUBLE PRECISION NOT NULL DEFAULT 0,
    platform_analytics   jsonb,
    sync_status          TEXT,
    metrics_last_updated TIMESTAMPTZ,
    raw_json             TEXT,
    occurred_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
        CREATE EXTENSION IF NOT EXISTS timescaledb;
        PERFORM create_hypertable('post_analytics_snapshots_fat', 'occurred_at',
            chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);
        ALTER TABLE post_analytics_snapshots_fat SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'tenant_id'
        );
        PERFORM add_compression_policy('post_analytics_snapshots_fat', INTERVAL '7 days', if_not_exists => TRUE);
        PERFORM add_retention_policy('post_analytics_snapshots_fat', INTERVAL '90 days', if_not_exists => TRUE);
    END IF;
END $$;

-- Re-denormalise the static fields from post_analytics_current back onto each
-- history row; raw_json is unrecoverable, defaulted to '{}'.
INSERT INTO post_analytics_snapshots_fat
    (id, tenant_id, post_id, publisher_post_id, publisher, platform, title, published_at,
     impressions, reach, likes, comments, shares, saves, clicks, views, engagement_rate,
     platform_analytics, sync_status, metrics_last_updated, raw_json, occurred_at)
SELECT s.id, s.tenant_id, s.post_id,
       COALESCE(c.publisher_post_id, ''), COALESCE(c.publisher, ''),
       c.platform, c.title, c.published_at,
       s.impressions, s.reach, s.likes, s.comments, s.shares, s.saves, s.clicks, s.views, s.engagement_rate,
       COALESCE(c.platform_analytics, '[]'::jsonb), s.sync_status, s.metrics_last_updated, '{}', s.occurred_at
FROM post_analytics_snapshots s
LEFT JOIN post_analytics_current c
       ON c.tenant_id = s.tenant_id AND c.post_id = s.post_id;

-- 2. Swap the fat table back into place.
DROP TABLE post_analytics_snapshots;
ALTER TABLE post_analytics_snapshots_fat RENAME TO post_analytics_snapshots;
CREATE INDEX idx_post_analytics_snapshots_post_time   ON post_analytics_snapshots (post_id, occurred_at DESC);
CREATE INDEX idx_post_analytics_snapshots_tenant_time ON post_analytics_snapshots (tenant_id, occurred_at DESC);

-- 3. Drop the current-state table.
DROP TABLE post_analytics_current;
