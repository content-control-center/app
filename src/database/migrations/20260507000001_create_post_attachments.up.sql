-- post_attachments holds one row per image bound to a Post (CON-73).
-- Attachments are post-scoped (FK + cascade) and ordered via `position`,
-- which is unique per post. Per-platform constraints (size/format/animated)
-- live in src/platforms/constraints.go and are evaluated at request time
-- and at publish time; the table itself only stores raw metadata.
CREATE TABLE post_attachments
(
    id              TEXT     PRIMARY KEY,
    post_id         TEXT     NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    position        INTEGER  NOT NULL,
    mime_type       TEXT     NOT NULL,
    size_bytes      INTEGER  NOT NULL,
    width           INTEGER  NOT NULL DEFAULT 0,
    height          INTEGER  NOT NULL DEFAULT 0,
    is_animated     INTEGER  NOT NULL DEFAULT 0,
    checksum_sha256 TEXT     NOT NULL,
    s3_key          TEXT     NOT NULL,
    created_by      TEXT     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (post_id, position)
);
CREATE INDEX idx_post_attachments_post_id ON post_attachments (post_id);
