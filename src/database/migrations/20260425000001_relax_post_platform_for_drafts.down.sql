-- Reverse of the platform-relaxation migration. Recreates posts with
-- platform_id NOT NULL ON DELETE CASCADE and platform_post_type NOT NULL,
-- matching the original schema after the cumulative ADD/RENAME column
-- migrations. Any rows whose platform_id became NULL while constraints
-- were relaxed will block this rollback — the operator must backfill or
-- delete those rows before running it.

PRAGMA foreign_keys = OFF;

CREATE TABLE posts_new
(
    id                       TEXT     PRIMARY KEY,
    campaign_id              TEXT     NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    platform_id              TEXT     NOT NULL REFERENCES platforms (id) ON DELETE CASCADE,
    platform_post_type       TEXT     NOT NULL,
    title                    TEXT     NOT NULL,
    content                  TEXT     NOT NULL DEFAULT '',
    media_urls               TEXT     NOT NULL DEFAULT '[]',
    scheduled_at             DATETIME,
    published_at             DATETIME,
    status                   TEXT     NOT NULL DEFAULT 'draft',
    cta_type                 TEXT     NOT NULL DEFAULT 'none',
    cta_url                  TEXT     NOT NULL DEFAULT '',
    target_audience_notes    TEXT     NOT NULL DEFAULT '',
    used_asset_ids           TEXT     NOT NULL DEFAULT '[]',
    campaign_type_phase_id   TEXT              REFERENCES campaigns_types_phases (id),
    created_by               TEXT     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO posts_new (
    id, campaign_id, platform_id, platform_post_type, title, content,
    media_urls, scheduled_at, published_at, status, cta_type, cta_url,
    target_audience_notes, used_asset_ids, campaign_type_phase_id,
    created_by, created_at, updated_at
)
SELECT
    id, campaign_id, platform_id, platform_post_type, title, content,
    media_urls, scheduled_at, published_at, status, cta_type, cta_url,
    target_audience_notes, used_asset_ids, campaign_type_phase_id,
    created_by, created_at, updated_at
FROM posts;

DROP TABLE posts;
ALTER TABLE posts_new RENAME TO posts;

PRAGMA foreign_keys = ON;
