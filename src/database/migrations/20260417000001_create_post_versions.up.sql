CREATE TABLE post_versions (
    id             TEXT     PRIMARY KEY,
    post_id        TEXT     NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    version_number INTEGER  NOT NULL,
    description    TEXT     NOT NULL,
    note           TEXT     NOT NULL DEFAULT '',
    creator        TEXT     NOT NULL CHECK (creator IN ('assistant', 'user')),
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_post_versions_post_id ON post_versions (post_id);
CREATE UNIQUE INDEX idx_post_versions_post_version ON post_versions (post_id, version_number);
