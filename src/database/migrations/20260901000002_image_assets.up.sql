-- CON-246: Image in Assets. Let an image be a first-class Content Bank asset
-- (type = 'IMG'), uploaded through the same batch endpoint as .md/.pdf, probed
-- and stored like a PDF's original. Three changes:
--   1. allow the new 'IMG' asset type,
--   2. add assets.alt_text (a short accessibility string, distinct from the
--      content description),
--   3. give asset_files the image-shaped columns post_attachments already
--      carries (width/height/is_animated/checksum_sha256), so the future
--      attach-to-post bridge is a field copy rather than a translation.
--
-- Additively compatible with CON-105 (PR #66), which layers ai_generated /
-- brand_style / generation on top of the same 'IMG' type.

-- 1. Extend the asset type CHECK to include 'IMG' (CON-222 named it
--    assets_type_check when it added 'URL').
ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_type_check;
ALTER TABLE assets
    ADD CONSTRAINT assets_type_check CHECK (type IS NULL OR type IN ('MD', 'PDF', 'URL', 'IMG'));

-- 2. Alt text lives on the asset (not asset_files): it describes the asset, and
--    a future multi-file asset still has one. Distinct from content, which holds
--    the longer, embeddable description.
ALTER TABLE assets ADD COLUMN alt_text TEXT NOT NULL DEFAULT '';

-- 3. Image metadata on the file row. Same names/types as post_attachments so the
--    attach path copies fields straight across. Defaulted so existing PDF rows
--    backfill silently; checksum is nullable (older rows have none).
ALTER TABLE asset_files
    ADD COLUMN width           INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN height          INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN is_animated     BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN checksum_sha256 TEXT;

-- Dedupe support: an identical file (same tenant, same non-empty checksum) is
-- the same asset. UNIQUE so the guarantee holds even under a race between two
-- concurrent identical uploads — the handler catches the conflict and returns
-- the existing asset rather than failing or duplicating.
--
-- Partial AND empty-excluding: only probed image rows carry a real checksum. PDF
-- rows leave it NULL (rows predating this migration) or '' (the worker's
-- zero-value insert), so both are excluded — otherwise every checksum-less PDF
-- in a tenant would collide with the next.
CREATE UNIQUE INDEX idx_asset_files_tenant_checksum
    ON asset_files (tenant_id, checksum_sha256)
    WHERE checksum_sha256 IS NOT NULL AND checksum_sha256 <> '';
