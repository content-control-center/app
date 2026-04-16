DROP TABLE asset_files;

PRAGMA foreign_keys = OFF;

CREATE TABLE assets_new
(
    id         TEXT     PRIMARY KEY,
    title      TEXT     NOT NULL,
    content    TEXT     NOT NULL,
    status     TEXT     NOT NULL DEFAULT 'ready'
        CHECK (status IN ('pending', 'processing', 'ready', 'failed')),
    type       TEXT
        CHECK (type IS NULL OR type IN ('MD')),
    tag_ids    TEXT     NOT NULL,
    created_by TEXT     NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Drop rows that would violate the narrower CHECKs so the copy succeeds.
DELETE FROM assets WHERE status = 'partial' OR type = 'PDF';

INSERT INTO assets_new (id, title, content, status, type, tag_ids, created_by, created_at, updated_at)
SELECT id, title, content, status, type, tag_ids, created_by, created_at, updated_at
FROM assets;

DROP TABLE assets;
ALTER TABLE assets_new RENAME TO assets;

PRAGMA foreign_keys = ON;
