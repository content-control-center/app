-- CON-245: reverse brand bindings (indexes drop with their columns).
ALTER TABLE posts     DROP COLUMN IF EXISTS brand_audience_id;
ALTER TABLE posts     DROP COLUMN IF EXISTS brand_voice_id;
ALTER TABLE campaigns DROP COLUMN IF EXISTS brand_audience_id;
ALTER TABLE campaigns DROP COLUMN IF EXISTS brand_voice_id;
