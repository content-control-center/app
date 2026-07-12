-- CON-105: image-generation support on assets.
--
-- Three column additions for Nano Banana image generation:
--   * ai_generated — marks assets produced by image generation, for the
--     GDPR / EU AI Act AI-generated disclosure surface (§10).
--   * brand_style  — marks curated brand-kit reference assets that supply the
--     style role during generation (§6). A small, tenant-scoped set.
--   * generation   — jsonb reproducibility + provenance metadata for a
--     generated asset (model id, aspect ratio, resolution, SynthID / C2PA);
--     NULL for non-generated assets. The cost snapshot itself lives in
--     usage_events (§9), keyed by tenant/model/time — not duplicated here.
-- Plus a new 'IMG' asset type so generated images sit alongside MD/PDF uploads.
ALTER TABLE assets
    ADD COLUMN ai_generated BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN brand_style  BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN generation   jsonb;

ALTER TABLE assets
    DROP CONSTRAINT IF EXISTS assets_type_check,
    ADD  CONSTRAINT assets_type_check CHECK (type IS NULL OR type IN ('MD', 'PDF', 'IMG'));

-- Brand-kit selection reads "this tenant's brand_style assets"; a partial index
-- keyed by tenant keeps that lookup cheap while indexing only the flagged rows.
CREATE INDEX idx_assets_brand_style ON assets (tenant_id) WHERE brand_style;
