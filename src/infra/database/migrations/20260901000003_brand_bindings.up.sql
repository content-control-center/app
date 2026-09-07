-- CON-245: bind Brand voice/audience to campaigns and posts. Nullable FKs,
-- ON DELETE SET NULL. Indexed for resolve joins and usage counts.
ALTER TABLE campaigns ADD COLUMN brand_voice_id TEXT REFERENCES brand_voices (id) ON DELETE SET NULL;
ALTER TABLE campaigns ADD COLUMN brand_audience_id TEXT REFERENCES brand_audiences (id) ON DELETE SET NULL;
ALTER TABLE posts ADD COLUMN brand_voice_id TEXT REFERENCES brand_voices (id) ON DELETE SET NULL;
ALTER TABLE posts ADD COLUMN brand_audience_id TEXT REFERENCES brand_audiences (id) ON DELETE SET NULL;
CREATE INDEX idx_campaigns_brand_voice_id ON campaigns (brand_voice_id);
CREATE INDEX idx_campaigns_brand_audience_id ON campaigns (brand_audience_id);
CREATE INDEX idx_posts_brand_voice_id ON posts (brand_voice_id);
CREATE INDEX idx_posts_brand_audience_id ON posts (brand_audience_id);
