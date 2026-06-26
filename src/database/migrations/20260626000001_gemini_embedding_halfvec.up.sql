-- CON-101: move asset-chunk embeddings from the self-hosted embeddinggemma-300m
-- model (768 dims) to the hosted Gemini Embedding 2 model (3072 dims).
--
-- pgvector's full-precision `vector` HNSW index caps at 2000 dimensions, so the
-- column becomes `halfvec(3072)` — 16-bit floats, indexable up to 4000 dims.
-- Gemini's native 3072-dim output is L2-normalized, so cosine search via the
-- `<=>` operator continues to work without renormalization.
--
-- Existing 768-dim vectors cannot be cast to 3072 dims, so they are dropped
-- (set to NULL). Chunks with a NULL embedding are excluded from search
-- (SearchSimilar filters `embedding IS NOT NULL`) and are re-embedded with
-- Gemini the next time their asset is saved or re-uploaded ("new content only").

DROP INDEX IF EXISTS idx_assets_chunks_embedding;

ALTER TABLE assets_chunks
    ALTER COLUMN embedding TYPE halfvec(3072) USING NULL::halfvec(3072);

-- HNSW index for cosine-distance ANN search (embedding <=> query).
CREATE INDEX idx_assets_chunks_embedding ON assets_chunks USING hnsw (embedding halfvec_cosine_ops);
