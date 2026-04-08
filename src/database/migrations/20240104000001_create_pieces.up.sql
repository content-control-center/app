CREATE TABLE pieces
(
    id         TEXT     PRIMARY KEY,
    title      TEXT     NOT NULL,
    content    TEXT     NOT NULL,
    created_by TEXT     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
