-- Recreate the legacy post_analytics table (schema only — the historical rows
-- are NOT restored; they live in the analytics DB post_analytics_snapshots).
-- This mirrors the table as it stood after the baseline schema plus the
-- domain_tenant_id migration: tenant_id NOT NULL with an FK and its index.
CREATE TABLE post_analytics (
    post_id              TEXT             PRIMARY KEY REFERENCES posts (id) ON DELETE CASCADE,
    publisher_post_id    TEXT             NOT NULL UNIQUE,
    impressions          INTEGER          NOT NULL DEFAULT 0,
    reach                INTEGER          NOT NULL DEFAULT 0,
    likes                INTEGER          NOT NULL DEFAULT 0,
    comments             INTEGER          NOT NULL DEFAULT 0,
    shares               INTEGER          NOT NULL DEFAULT 0,
    saves                INTEGER          NOT NULL DEFAULT 0,
    clicks               INTEGER          NOT NULL DEFAULT 0,
    views                INTEGER          NOT NULL DEFAULT 0,
    engagement_rate      DOUBLE PRECISION NOT NULL DEFAULT 0,
    platform_analytics   jsonb            NOT NULL DEFAULT '[]',
    sync_status          TEXT             NOT NULL DEFAULT '',
    metrics_last_updated TIMESTAMPTZ,
    last_refreshed_at    TIMESTAMPTZ      NOT NULL DEFAULT now(),
    raw_json             TEXT             NOT NULL DEFAULT '{}',
    created_at           TIMESTAMPTZ      NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ      NOT NULL DEFAULT now(),
    tenant_id            TEXT             NOT NULL REFERENCES tenants (id)
);
CREATE INDEX idx_post_analytics_tenant_id ON post_analytics (tenant_id);
