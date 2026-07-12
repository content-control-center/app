-- CON-105 down: remove image-generation support from assets. Rolling this back
-- requires no assets of type 'IMG' to remain, since the restored CHECK admits
-- only MD/PDF.
DROP INDEX IF EXISTS idx_assets_brand_style;

ALTER TABLE assets
    DROP CONSTRAINT IF EXISTS assets_type_check,
    ADD  CONSTRAINT assets_type_check CHECK (type IS NULL OR type IN ('MD', 'PDF'));

ALTER TABLE assets
    DROP COLUMN IF EXISTS generation,
    DROP COLUMN IF EXISTS brand_style,
    DROP COLUMN IF EXISTS ai_generated;
