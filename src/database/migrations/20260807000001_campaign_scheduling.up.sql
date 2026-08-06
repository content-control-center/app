-- CON-181: per-campaign scheduling settings consumed by the content-plan flow.
-- publishing_time is a local "HH:MM" wall clock; timezone is an IANA name
-- ('' = UTC); publishing_days is the enabled weekday set; spread_minutes is the
-- ± jitter applied around the publishing time. Column defaults backfill existing
-- campaigns to 09:00 / UTC / every day / ±15 min.
ALTER TABLE campaigns
    ADD COLUMN publishing_time TEXT    NOT NULL DEFAULT '09:00',
    ADD COLUMN timezone        TEXT    NOT NULL DEFAULT '',
    ADD COLUMN publishing_days jsonb   NOT NULL DEFAULT '["mon","tue","wed","thu","fri","sat","sun"]',
    ADD COLUMN spread_minutes  INTEGER NOT NULL DEFAULT 15;
