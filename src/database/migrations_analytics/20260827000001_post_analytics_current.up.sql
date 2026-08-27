-- CON-236: shrink post_analytics storage. The refresh job used to append a full
-- row per post PER TICK (every 30m) with no change detection, so ~9 posts
-- produced 9,000+ near-duplicate rows. This migration splits the data into:
--
--   post_analytics_current   — ONE row per post (upserted every check); holds the
--                              current metrics + denormalised post display fields
--                              + latest per-platform breakdown. Serves every
--                              analytics read as a plain indexed scan.
--   post_analytics_snapshots — slimmed to metric columns only, written ONLY when
--                              the metrics change (dedup). The trend history /
--                              archive; retention-pruned. raw_json is dropped
--                              entirely (it was never read).
--
-- The existing fat post_analytics_snapshots is the backfill source; we rebuild
-- it slim via a build-new + swap so we never run column-drop DDL against a
-- compressed hypertable (version-fragile) and can retroactively compact the
-- historical duplicates in the same pass.

-- 1. Current-state table: one row per post, the read source.
CREATE TABLE post_analytics_current (
    tenant_id            TEXT             NOT NULL,
    post_id              TEXT             NOT NULL,
    publisher            TEXT             NOT NULL,
    publisher_post_id    TEXT             NOT NULL,
    platform             TEXT,                              -- denormalised — was platforms.name
    title                TEXT,                              -- denormalised — was posts.title
    published_at         TIMESTAMPTZ,                       -- denormalised — was posts.published_at
    impressions          BIGINT           NOT NULL DEFAULT 0,
    reach                BIGINT           NOT NULL DEFAULT 0,
    likes                BIGINT           NOT NULL DEFAULT 0,
    comments             BIGINT           NOT NULL DEFAULT 0,
    shares               BIGINT           NOT NULL DEFAULT 0,
    saves                BIGINT           NOT NULL DEFAULT 0,
    clicks               BIGINT           NOT NULL DEFAULT 0,
    views                BIGINT           NOT NULL DEFAULT 0,
    engagement_rate      DOUBLE PRECISION NOT NULL DEFAULT 0,
    platform_analytics   jsonb            NOT NULL DEFAULT '[]',   -- LATEST per-platform breakdown
    sync_status          TEXT,
    metrics_last_updated TIMESTAMPTZ,                       -- publisher compute time (staleness)
    first_seen_at        TIMESTAMPTZ      NOT NULL DEFAULT now(),  -- first snapshot ever recorded
    last_changed_at      TIMESTAMPTZ      NOT NULL DEFAULT now(),  -- metrics last actually moved
    last_checked_at      TIMESTAMPTZ      NOT NULL DEFAULT now()   -- refresh last looked (freshness)
);

-- One row per post; also the ON CONFLICT target for the per-check upsert.
CREATE UNIQUE INDEX idx_post_analytics_current_uniq ON post_analytics_current (tenant_id, post_id);
-- The overview scans by tenant + publisher (and optionally platform).
CREATE INDEX idx_post_analytics_current_tenant_pub ON post_analytics_current (tenant_id, publisher);

-- Backfill current from the latest snapshot per post in the existing fat table.
-- last_changed_at is the occurred_at of the last row whose metric key differed
-- from its predecessor (the same LAG comparison used to compact the history
-- below), NOT the latest observation — which stays as last_checked_at. Every
-- post has at least one such row (its first observation), so the join always
-- matches.
WITH marked AS (
    SELECT s.tenant_id, s.post_id, s.occurred_at,
        LAG(ROW(impressions, reach, likes, comments, shares, saves, clicks, views, engagement_rate, sync_status))
            OVER (PARTITION BY tenant_id, post_id ORDER BY occurred_at, id) AS prev_key,
        ROW(impressions, reach, likes, comments, shares, saves, clicks, views, engagement_rate, sync_status) AS cur_key
    FROM post_analytics_snapshots s
),
last_change AS (
    SELECT tenant_id, post_id, MAX(occurred_at) AS last_changed_at
    FROM marked
    WHERE prev_key IS DISTINCT FROM cur_key
    GROUP BY tenant_id, post_id
)
INSERT INTO post_analytics_current
    (tenant_id, post_id, publisher, publisher_post_id, platform, title, published_at,
     impressions, reach, likes, comments, shares, saves, clicks, views, engagement_rate,
     platform_analytics, sync_status, metrics_last_updated,
     first_seen_at, last_changed_at, last_checked_at)
SELECT DISTINCT ON (s.tenant_id, s.post_id)
    s.tenant_id, s.post_id, s.publisher, s.publisher_post_id, s.platform, s.title, s.published_at,
    s.impressions, s.reach, s.likes, s.comments, s.shares, s.saves, s.clicks, s.views, s.engagement_rate,
    COALESCE(s.platform_analytics, '[]'::jsonb), s.sync_status, s.metrics_last_updated,
    MIN(s.occurred_at) OVER (PARTITION BY s.tenant_id, s.post_id),  -- first_seen_at
    lc.last_changed_at,                                            -- last_changed_at (last real change)
    s.occurred_at                                                  -- last_checked_at (latest observation)
FROM post_analytics_snapshots s
JOIN last_change lc ON lc.tenant_id = s.tenant_id AND lc.post_id = s.post_id
ORDER BY s.tenant_id, s.post_id, s.occurred_at DESC, s.id DESC;

-- 2. Slim history table (build-new). Same name suffixed _v2; swapped in below.
CREATE TABLE post_analytics_snapshots_v2 (
    id                   TEXT             NOT NULL,
    tenant_id            TEXT             NOT NULL,
    post_id              TEXT             NOT NULL,
    impressions          BIGINT           NOT NULL DEFAULT 0,
    reach                BIGINT           NOT NULL DEFAULT 0,
    likes                BIGINT           NOT NULL DEFAULT 0,
    comments             BIGINT           NOT NULL DEFAULT 0,
    shares               BIGINT           NOT NULL DEFAULT 0,
    saves                BIGINT           NOT NULL DEFAULT 0,
    clicks               BIGINT           NOT NULL DEFAULT 0,
    views                BIGINT           NOT NULL DEFAULT 0,
    engagement_rate      DOUBLE PRECISION NOT NULL DEFAULT 0,
    sync_status          TEXT,
    metrics_last_updated TIMESTAMPTZ,
    occurred_at          TIMESTAMPTZ      NOT NULL DEFAULT now()
);

-- Promote to a hypertable BEFORE backfill (create_hypertable wants an empty
-- table unless migrate_data). Guarded so vanilla Postgres runs the plain table.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
        CREATE EXTENSION IF NOT EXISTS timescaledb;
        PERFORM create_hypertable('post_analytics_snapshots_v2', 'occurred_at',
            chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);
        ALTER TABLE post_analytics_snapshots_v2 SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'tenant_id'
        );
        PERFORM add_compression_policy('post_analytics_snapshots_v2', INTERVAL '7 days', if_not_exists => TRUE);
        PERFORM add_retention_policy('post_analytics_snapshots_v2', INTERVAL '90 days', if_not_exists => TRUE);
    END IF;
END $$;

-- Retroactively compact the historical duplicates: keep a row only where its
-- metric key differs from the chronologically previous row for the same post
-- (the first row per post has a NULL predecessor, so it is always kept).
INSERT INTO post_analytics_snapshots_v2
    (id, tenant_id, post_id, impressions, reach, likes, comments, shares, saves, clicks, views,
     engagement_rate, sync_status, metrics_last_updated, occurred_at)
SELECT id, tenant_id, post_id, impressions, reach, likes, comments, shares, saves, clicks, views,
       engagement_rate, sync_status, metrics_last_updated, occurred_at
FROM (
    SELECT s.*,
        LAG(ROW(impressions, reach, likes, comments, shares, saves, clicks, views, engagement_rate, sync_status))
            OVER (PARTITION BY tenant_id, post_id ORDER BY occurred_at, id) AS prev_key
    FROM post_analytics_snapshots s
) d
WHERE prev_key IS DISTINCT FROM
      ROW(impressions, reach, likes, comments, shares, saves, clicks, views, engagement_rate, sync_status);

-- 3. Swap the slim table into place.
DROP TABLE post_analytics_snapshots;
ALTER TABLE post_analytics_snapshots_v2 RENAME TO post_analytics_snapshots;

-- Recreate the canonical indexes (names freed by the DROP above).
CREATE INDEX idx_post_analytics_snapshots_post_time   ON post_analytics_snapshots (post_id, occurred_at DESC);
CREATE INDEX idx_post_analytics_snapshots_tenant_time ON post_analytics_snapshots (tenant_id, occurred_at DESC);
