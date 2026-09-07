-- CON-86: usage_events lives in the ISOLATED analytics database (CON-86 D4).
--
-- It is created as a plain table first so this migration runs unchanged on
-- vanilla Postgres (tests / dev without TimescaleDB). The TimescaleDB upgrade
-- below is guarded by an extension-availability check, so the same file turns
-- the table into a hypertable with compression + retention only where the
-- extension is installed.
--
-- No FOREIGN KEY on tenant_id: this database is isolated and has no tenants
-- table. Tenant isolation is enforced app-side by the TenantScoped hooks.
CREATE TABLE usage_events (
    id                    TEXT        NOT NULL,
    tenant_id             TEXT        NOT NULL,
    vendor                TEXT        NOT NULL,
    family                TEXT        NOT NULL,
    model                 TEXT        NOT NULL,
    operation             TEXT        NOT NULL,
    feature               TEXT        NOT NULL,
    platform              TEXT,
    social_account_id     TEXT,
    input_tokens          BIGINT      NOT NULL DEFAULT 0,
    output_tokens         BIGINT      NOT NULL DEFAULT 0,
    cache_read_tokens     BIGINT      NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT      NOT NULL DEFAULT 0,
    extra_units           jsonb,
    cost_micros           BIGINT      NOT NULL DEFAULT 0,
    price_version         TEXT,
    occurred_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Period-to-date spend (enforcement) and the usage summary both filter by
-- tenant over a time window; the breakdown also slices by vendor.
CREATE INDEX idx_usage_events_tenant_time        ON usage_events (tenant_id, occurred_at DESC);
CREATE INDEX idx_usage_events_tenant_vendor_time ON usage_events (tenant_id, vendor, occurred_at DESC);

-- TimescaleDB upgrade — applied only where the extension is available, so the
-- statements above stand alone on vanilla Postgres (CON-86 FR6). The daily
-- continuous aggregate is intentionally deferred: the repository computes
-- period spend / summaries directly off this table (correct with or without
-- Timescale); the hypertable just makes those scans cheap at volume.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
        CREATE EXTENSION IF NOT EXISTS timescaledb;

        PERFORM create_hypertable('usage_events', 'occurred_at',
            chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);

        ALTER TABLE usage_events SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'tenant_id'
        );
        PERFORM add_compression_policy('usage_events', INTERVAL '7 days', if_not_exists => TRUE);

        -- Default 90-day retention mirrors PostLogRetentionDays. Operators who
        -- raise USAGE_RETENTION_DAYS adjust this policy out of band.
        PERFORM add_retention_policy('usage_events', INTERVAL '90 days', if_not_exists => TRUE);
    END IF;
END $$;
