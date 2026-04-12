CREATE TABLE platforms
(
    id         TEXT     PRIMARY KEY,
    name       TEXT     NOT NULL,
    post_types TEXT     NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
