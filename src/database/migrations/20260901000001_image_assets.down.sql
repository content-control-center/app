-- CON-246 down: remove image-asset support. IMG assets (and their cascaded
-- files/chunks) are deleted first so the reinstated 'MD'/'PDF'/'URL'-only CHECK
-- holds.
DELETE FROM assets WHERE type = 'IMG';

DROP INDEX IF EXISTS idx_asset_files_tenant_checksum;

ALTER TABLE asset_files
    DROP COLUMN IF EXISTS width,
    DROP COLUMN IF EXISTS height,
    DROP COLUMN IF EXISTS is_animated,
    DROP COLUMN IF EXISTS checksum_sha256;

ALTER TABLE assets DROP COLUMN IF EXISTS alt_text;

ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_type_check;
ALTER TABLE assets
    ADD CONSTRAINT assets_type_check CHECK (type IS NULL OR type IN ('MD', 'PDF', 'URL'));
