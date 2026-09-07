-- CON-246 down: remove image-asset support (reinstate the 'MD'/'PDF'/'URL'-only
-- type CHECK and drop the image columns/index).
--
-- This migration created no rows — it only enabled the 'IMG' type — so every IMG
-- asset is user data (an upload) or a CON-105 generated image. The rollback must
-- not destroy either. Instead of deleting them, it ABORTS when any IMG asset
-- exists: an operator has to consciously remove or reclassify them before the
-- schema can be rolled back. The guard runs first, so nothing below executes on
-- the abort path (the rollback is all-or-nothing regardless of transaction
-- wrapping), and it gives a clear reason rather than a raw CHECK violation.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM assets WHERE type = 'IMG') THEN
        RAISE EXCEPTION
            'cannot roll back 20260901000002_image_assets: % IMG asset(s) exist; remove or reclassify them before down-migrating (rollback refuses to delete user / CON-105 image data)',
            (SELECT count(*) FROM assets WHERE type = 'IMG');
    END IF;
END $$;

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
