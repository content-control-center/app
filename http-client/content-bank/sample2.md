# PDF Piece Support: Text Extraction & Chunk-Based Embedding

## Summary

Add support for PDF files as Pieces in Ogen. PDFs must be converted to text before vector embedding. Due to the size of typical PDFs (80+ pages), a chunk-based embedding strategy is required to maintain retrieval precision within the existing ~3,000 token context budget.

---

## 1. PDF Text Extraction

**Primary:** `ledongthuc/pdf` — pure Go, no CGo, no system dependencies. Handles standard text-based PDFs well.

**Fallback:** Shell out to `pdftotext` (from `poppler-utils`) for PDFs where the primary extractor returns empty or suspiciously short output relative to page count.

**Limitation:** Neither handles scanned/image-only PDFs (no OCR). If OCR becomes a requirement, evaluate `gen2brain/go-fitz` (CGo, wraps MuPDF, AGPL-licensed).

---

## 2. Chunking Strategy

Extract text page-by-page, then split into chunks targeting **~500–800 tokens** (~2,000–3,200 characters).

**Algorithm:**
- Split each page's text into paragraphs (double newline)
- Accumulate paragraphs into a chunk until the token threshold is reached
- Close the chunk, carry the last paragraph into the next chunk as overlap (~100–200 tokens)
- Track page boundaries per chunk for citation metadata

**Why this range:** 500–800 tokens balances retrieval precision (specific enough for meaningful similarity matches) with context coherence (enough surrounding text to be useful). An 80-page PDF produces ~120–200 chunks at this size.

---

## 3. Schema Migration

### Current state

`pieces_embeddings` has `piece_id` as PRIMARY KEY — strictly 1:1 with Pieces. Cannot store multiple chunks per Piece.

### New table: `piece_chunks`

```sql
CREATE TABLE piece_chunks (
    id          TEXT     PRIMARY KEY,
    piece_id    TEXT     NOT NULL REFERENCES pieces(id) ON DELETE CASCADE,
    chunk_index INTEGER  NOT NULL,
    page_start  INTEGER,
    page_end    INTEGER,
    content     TEXT     NOT NULL,
    token_count INTEGER  NOT NULL,
    embedding   BLOB,
    model       TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(piece_id, chunk_index)
);

CREATE INDEX idx_piece_chunks_piece_id ON piece_chunks(piece_id);
```

Embedding is stored alongside chunk content (no separate embeddings table for chunks) — retrieval always needs both, so avoiding an extra join on every vector search.

### Unified migration (recommended)

Migrate existing `pieces_embeddings` rows into `piece_chunks` as single-chunk entries (`chunk_index=0`, no page info), then deprecate `pieces_embeddings`. This gives one retrieval path instead of branching logic.

```sql
INSERT INTO piece_chunks (id, piece_id, chunk_index, content, token_count, embedding, model)
SELECT
    piece_id || ':0',
    piece_id,
    0,
    '',       -- backfill from pieces table
    0,        -- backfill token count
    embedding,
    model
FROM pieces_embeddings;
```

Backfill `content` and `token_count` from the `pieces` table in a second pass.

---

## 4. Retrieval Query (for generateContentPlan)

```sql
SELECT pc.piece_id, pc.chunk_index, pc.page_start, pc.page_end,
       pc.content, pc.token_count
FROM piece_chunks pc
WHERE pc.piece_id IN (?)
ORDER BY vec_distance(pc.embedding, ?) ASC
LIMIT 20;
```

Then in Go, greedily fill the token budget:

```go
budget := 3000
for _, chunk := range rankedChunks {
    if budget - chunk.TokenCount < 0 {
        break
    }
    selected = append(selected, chunk)
    budget -= chunk.TokenCount
}
// Sort selected by (piece_id, chunk_index) for coherent ordering
```

---

## 5. PDF Ingestion Flow

```
Upload PDF
  → Extract text per page (ledongthuc/pdf, fallback to pdftotext)
  → Split into chunks (~600 tokens, paragraph-aware, with overlap)
  → For each chunk: compute embedding, INSERT into piece_chunks
  → Piece row stores metadata (filename, page_count, source_type="pdf")
```

---

## Open Questions

1. Do we need OCR support for scanned PDFs now, or defer?
2. Should chunk size be configurable per Piece, or fixed globally?
3. Should section-aware splitting (detecting headings) be implemented for structured documents (reports, whitepapers)?
