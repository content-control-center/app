-- Expand assets.status CHECK to include 'partial' and assets.type CHECK to
-- include 'PDF'. SQLite can't ALTER a CHECK constraint in place, so the
-- table is rebuilt.

PRAGMA foreign_keys = OFF;

CREATE TABLE assets_new
(
    id         TEXT     PRIMARY KEY,
    title      TEXT     NOT NULL,
    content    TEXT     NOT NULL,
    status     TEXT     NOT NULL DEFAULT 'ready'
        CHECK (status IN ('pending', 'processing', 'ready', 'partial', 'failed')),
    type       TEXT
        CHECK (type IS NULL OR type IN ('MD', 'PDF')),
    tag_ids    TEXT     NOT NULL,
    created_by TEXT     NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO assets_new (id, title, content, status, type, tag_ids, created_by, created_at, updated_at)
SELECT id, title, content, status, type, tag_ids, created_by, created_at, updated_at
FROM assets;

DROP TABLE assets;
ALTER TABLE assets_new RENAME TO assets;

PRAGMA foreign_keys = ON;

-- One-to-one metadata for file-backed Assets (PDF uploads today; extensible).
CREATE TABLE asset_files
(
    id               TEXT     PRIMARY KEY,
    asset_id         TEXT     NOT NULL UNIQUE REFERENCES assets (id) ON DELETE CASCADE,
    original_name    TEXT     NOT NULL,
    mime_type        TEXT     NOT NULL,
    size_bytes       INTEGER  NOT NULL,
    s3_key           TEXT     NOT NULL,
    thumbnail_s3_key TEXT,
    page_count       INTEGER,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_asset_files_asset_id ON asset_files (asset_id);
