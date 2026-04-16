CREATE TABLE assets_embeddings
(
    asset_id   TEXT     PRIMARY KEY,
    embedding  BLOB     NOT NULL,
    model      TEXT     NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Backfill from chunk_index = 0 rows (single-chunk entries only).
INSERT INTO assets_embeddings (asset_id, embedding, model)
SELECT asset_id, embedding, model
FROM assets_chunks
WHERE chunk_index = 0
  AND embedding IS NOT NULL;

DROP INDEX IF EXISTS idx_assets_chunks_asset_id;
DROP TABLE assets_chunks;
