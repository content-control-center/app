CREATE TABLE pieces_embeddings
(
    piece_id   TEXT     PRIMARY KEY REFERENCES pieces (id) ON DELETE CASCADE,
    embedding  BLOB     NOT NULL,
    model      TEXT     NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
