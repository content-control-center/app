-- CON-153: follower_stats_snapshots — a per-account, per-day follower-count
-- time series in the isolated analytics database. Zernio refreshes follower
-- counts once per day; the refresh_zernio_followers job records one row per
-- (account, day) so Ogen keeps a durable follower history and growth trend
-- beyond Zernio's own reporting window.
--
-- One row per (tenant, account, day): the daily upsert keys on
-- (tenant_id, social_account_id, point_date), so a re-run within the same day
-- refines the count in place rather than appending a duplicate.
--
-- Display fields (platform, username) are DENORMALISED onto each row: this
-- database is isolated from social_accounts, so reads can't join to them. No
-- FOREIGN KEY on tenant_id/social_account_id; tenant isolation is enforced
-- app-side by the TenantScoped hooks (CON-97), mirroring
-- post_analytics_snapshots.
CREATE TABLE follower_stats_snapshots (
    id                 TEXT             NOT NULL,
    tenant_id          TEXT             NOT NULL,
    social_account_id  TEXT             NOT NULL,
    platform           TEXT,                             -- denormalised — was social_accounts.platform
    username           TEXT,                             -- denormalised — was social_accounts.username
    followers          BIGINT           NOT NULL DEFAULT 0,
    growth             BIGINT           NOT NULL DEFAULT 0,   -- Zernio-computed, vs the window start
    growth_percentage  DOUBLE PRECISION NOT NULL DEFAULT 0,
    point_date         DATE             NOT NULL,         -- the calendar day of the count (query axis)
    raw_json           TEXT,
    recorded_at        TIMESTAMPTZ      NOT NULL DEFAULT now()  -- the refresh time
);

-- One row per account per day; also the ON CONFLICT target for the daily
-- upsert. It includes point_date so the unique index stays compatible with the
-- hypertable partition column selected below (Timescale requires every unique
-- index to include the partitioning column).
CREATE UNIQUE INDEX idx_follower_stats_snapshots_uniq
    ON follower_stats_snapshots (tenant_id, social_account_id, point_date);
-- Series reads hit (social_account_id, point_date DESC).
CREATE INDEX idx_follower_stats_snapshots_account_date
    ON follower_stats_snapshots (social_account_id, point_date DESC);

-- TimescaleDB upgrade — applied only where the extension is available, so the
-- statements above stand alone on vanilla Postgres (mirrors post_analytics_snapshots).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
        CREATE EXTENSION IF NOT EXISTS timescaledb;

        PERFORM create_hypertable('follower_stats_snapshots', 'point_date',
            chunk_time_interval => INTERVAL '30 days', if_not_exists => TRUE);

        ALTER TABLE follower_stats_snapshots SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'tenant_id'
        );
        PERFORM add_compression_policy('follower_stats_snapshots', INTERVAL '30 days', if_not_exists => TRUE);

        -- 365-day retention: follower history is low-volume and its long-range
        -- trend is the whole reason to snapshot it (ZERNIO_FOLLOWER_RETENTION_DAYS
        -- carries the same default; operators adjust the policy out of band).
        PERFORM add_retention_policy('follower_stats_snapshots', INTERVAL '365 days', if_not_exists => TRUE);
    END IF;
END $$;
