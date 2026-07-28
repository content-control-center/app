-- CON-125: activity_events lives in the ISOLATED analytics database, alongside
-- usage_events (CON-86 D4). It records meaningful user/tenant *behaviour*
-- (post created, signup, ai flow run, publish outcome, …) tagged by category,
-- as a complement to usage_events (which records vendor cost/tokens).
--
-- Created as a plain table first so this migration runs unchanged on vanilla
-- Postgres (tests / dev without TimescaleDB). The TimescaleDB upgrade below is
-- guarded by an extension-availability check, mirroring usage_events.
--
-- No FOREIGN KEY on tenant_id: this database is isolated and has no tenants
-- table. Tenant isolation is enforced app-side by the TenantScoped hooks.
CREATE TABLE activity_events (
    id           TEXT        NOT NULL,
    tenant_id    TEXT        NOT NULL,
    user_id      TEXT,                 -- NULL for system/background activities
    category     TEXT        NOT NULL, -- primary tag: post | authentication | ai_flow | campaign | publish | …
    type         TEXT        NOT NULL, -- specific verb: post_created, signup, content_plan, publish_failed …
    entity_type  TEXT,                 -- post | campaign | user | tenant | social_account | zernio_job …
    entity_id    TEXT,
    status       TEXT,                 -- outcome/new-state: success | failed | draft->scheduled …
    source       TEXT,                 -- api | assistant | job | system
    tags         jsonb,                -- optional secondary labels (JSON array)
    payload      jsonb,                -- activity-specific fields (status_from/to, platform, model, counts, latency_ms, error …)
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- All reads are tenant + time windowed; the category index serves the common
-- "activity of kind X for a tenant over a window" slice, the entity index the
-- "history of one entity" slice.
CREATE INDEX idx_activity_events_tenant_time          ON activity_events (tenant_id, occurred_at DESC);
CREATE INDEX idx_activity_events_tenant_category_time ON activity_events (tenant_id, category, occurred_at DESC);
CREATE INDEX idx_activity_events_tenant_entity_time   ON activity_events (tenant_id, entity_type, entity_id, occurred_at DESC);

-- TimescaleDB upgrade — applied only where the extension is available, so the
-- statements above stand alone on vanilla Postgres (mirrors usage_events).
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'timescaledb') THEN
        CREATE EXTENSION IF NOT EXISTS timescaledb;

        PERFORM create_hypertable('activity_events', 'occurred_at',
            chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);

        ALTER TABLE activity_events SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'tenant_id'
        );
        PERFORM add_compression_policy('activity_events', INTERVAL '7 days', if_not_exists => TRUE);

        -- Default 90-day retention mirrors UsageRetentionDays. Operators who
        -- raise ACTIVITY_RETENTION_DAYS adjust this policy out of band (CON-125).
        PERFORM add_retention_policy('activity_events', INTERVAL '90 days', if_not_exists => TRUE);
    END IF;
END $$;
