-- CON-222: URL as an Asset. A submitted URL is scraped to Markdown via
-- Firecrawl and persisted like an MD/PDF asset, with its page images mirrored
-- into object storage. Three changes:
--   1. allow the new 'URL' asset type,
--   2. add a nullable source_url (the origin URL) + a per-tenant dedupe index so
--      re-submitting a URL refreshes the same asset,
--   3. a one-to-many asset_images table for the mirrored images (asset_files is
--      one-to-one and can't hold N images).

-- 1. Extend the asset type CHECK to include 'URL' (baseline named it
--    assets_type_check via the inline column CHECK).
ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_type_check;
ALTER TABLE assets
    ADD CONSTRAINT assets_type_check CHECK (type IS NULL OR type IN ('MD', 'PDF', 'URL'));

-- 2. Origin URL + dedupe. Partial unique index: only URL assets carry a
--    source_url, and a tenant may hold each URL once (re-submit = refresh).
ALTER TABLE assets ADD COLUMN source_url TEXT;
CREATE UNIQUE INDEX idx_assets_tenant_source_url
    ON assets (tenant_id, source_url) WHERE source_url IS NOT NULL;

-- 3. Mirrored page images. Cascades with the asset; idx orders them within the
--    asset and (asset_id, idx) is unique so a refresh can replace them cleanly.
CREATE TABLE asset_images (
    id         TEXT        PRIMARY KEY,
    asset_id   TEXT        NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
    idx        INTEGER     NOT NULL,
    source_url TEXT        NOT NULL,
    s3_key     TEXT        NOT NULL,
    mime_type  TEXT        NOT NULL DEFAULT '',
    size_bytes BIGINT      NOT NULL DEFAULT 0,
    alt        TEXT,
    tenant_id  TEXT        NOT NULL REFERENCES tenants (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (asset_id, idx)
);

CREATE INDEX idx_asset_images_asset_id  ON asset_images (asset_id);
CREATE INDEX idx_asset_images_tenant_id ON asset_images (tenant_id);
