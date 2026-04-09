CREATE TABLE posts
(
    id                    TEXT     PRIMARY KEY,
    campaign_id           TEXT     NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    platform_id           TEXT     NOT NULL REFERENCES platforms (id) ON DELETE CASCADE,
    platform_post_type    TEXT     NOT NULL,
    title                 TEXT     NOT NULL,
    content               TEXT     NOT NULL DEFAULT '',
    media_urls            TEXT     NOT NULL DEFAULT '[]',
    scheduled_at          DATETIME,
    published_at          DATETIME,
    status                TEXT     NOT NULL DEFAULT 'draft',
    cta_type              TEXT     NOT NULL DEFAULT 'none',
    cta_url               TEXT     NOT NULL DEFAULT '',
    target_audience_notes TEXT     NOT NULL DEFAULT '',
    used_pieces_ids       TEXT     NOT NULL DEFAULT '[]',
    created_by            TEXT     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
