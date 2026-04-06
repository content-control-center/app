CREATE TABLE sessions
(
    id         TEXT     PRIMARY KEY,
    user_id    TEXT     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
