-- Reverse CON-101: return the embedding column to vector(768) + vector_cosine_ops.
-- The 3072-dim halfvec values cannot be cast back to 768 dims, so embeddings are
-- cleared (set to NULL); content is re-embedded on its next save.

DROP INDEX IF EXISTS idx_assets_chunks_embedding;

ALTER TABLE assets_chunks
    ALTER COLUMN embedding TYPE vector(768) USING NULL::vector(768);

CREATE INDEX idx_assets_chunks_embedding ON assets_chunks USING hnsw (embedding vector_cosine_ops);
