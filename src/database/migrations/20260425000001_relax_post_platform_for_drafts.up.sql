-- Relax platform_id and platform_post_type so draft posts can exist
-- without a platform chosen. Both fields become required again only when
-- the post moves out of "draft" status — that gate is enforced at the
-- handler layer, not by the schema.
--
-- SQLite cannot drop NOT NULL in place; the standard recipe is
-- table-recreate. The posts table has no indexes, so swapping is safe.
-- platform_id's FK action also flips from CASCADE to SET NULL: deleting
-- a platform should null out drafts that referenced it, not delete them.

PRAGMA foreign_keys = OFF;

CREATE TABLE posts_new
(
    id                       TEXT     PRIMARY KEY,
    campaign_id              TEXT     NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    platform_id              TEXT              REFERENCES platforms (id) ON DELETE SET NULL,
    platform_post_type       TEXT,
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
