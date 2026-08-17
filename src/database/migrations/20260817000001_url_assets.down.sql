-- CON-222 down: remove URL-asset support. URL assets (and their cascaded images
-- and chunks) are deleted first so the reinstated 'MD'/'PDF'-only CHECK holds.
DROP TABLE IF EXISTS asset_images;

DELETE FROM assets WHERE type = 'URL';

DROP INDEX IF EXISTS idx_assets_tenant_source_url;
ALTER TABLE assets DROP COLUMN IF EXISTS source_url;

ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_type_check;
ALTER TABLE assets
    ADD CONSTRAINT assets_type_check CHECK (type IS NULL OR type IN ('MD', 'PDF'));
