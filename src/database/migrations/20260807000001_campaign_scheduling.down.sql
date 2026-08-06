-- CON-181: drop the per-campaign scheduling settings.
ALTER TABLE campaigns
    DROP COLUMN IF EXISTS publishing_time,
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS publishing_days,
    DROP COLUMN IF EXISTS spread_minutes;
