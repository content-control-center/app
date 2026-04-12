CREATE TABLE tags
(
    id         TEXT     PRIMARY KEY,
    name       TEXT     NOT NULL,
    color      TEXT     NOT NULL DEFAULT '',
    created_by TEXT     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
