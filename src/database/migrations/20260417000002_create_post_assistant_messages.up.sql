CREATE TABLE post_assistant_messages (
    id         TEXT     PRIMARY KEY,
    post_id    TEXT     NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    role       TEXT     NOT NULL CHECK (role IN ('user', 'model')),
    content    TEXT     NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_post_assistant_messages_post_id ON post_assistant_messages (post_id);
