CREATE TABLE assets_chunks
(
    id          TEXT     PRIMARY KEY,
    asset_id    TEXT     NOT NULL REFERENCES assets (id) ON DELETE CASCADE,
    chunk_index INTEGER  NOT NULL,
    page_start  INTEGER,
    page_end    INTEGER,
    content     TEXT     NOT NULL DEFAULT '',
    token_count INTEGER  NOT NULL DEFAULT 0,
    embedding   BLOB,
    model       TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (asset_id, chunk_index)
);

CREATE INDEX idx_assets_chunks_asset_id ON assets_chunks (asset_id);

-- Migrate existing assets_embeddings rows as single-chunk entries.
INSERT INTO assets_chunks (id, asset_id, chunk_index, content, token_count, embedding, model)
SELECT asset_id || ':0',
       asset_id,
       0,
       '',
       0,
       embedding,
       model
FROM assets_embeddings;

DROP TABLE assets_embeddings;
