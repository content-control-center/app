# Content-Bank Assets, Images & Post Attachments

Covers `src/handlers/{assets,images,post_attachments}.go`, the asset/file/chunk/attachment
models, `src/storage/*`, and the async upload/embed/PDF pipelines.

All endpoints require auth (`401` otherwise). The auth middleware populates
`c.Locals("session")` with a `*models.Session`; `session.UserID` fills `created_by`.

---

## 1. Models

### Asset (`models.Asset`, table `assets`)

A content-bank item: editor content (BlockNote JSON, default when created via JSON), a
Markdown import, or a PDF import. `Tags`/`File` are hydrated (`bun:"-"`), not columns.

| JSON | Type | Notes |
|---|---|---|
| `id` | string | Sqid |
| `title` | string | notnull |
| `content` | string | BlockNote JSON for editor/MD; `"[]"` placeholder for PDF |
| `status` | string | enum, default `'ready'` |
| `type` | `*string` | nil (JSON-created), `"MD"`, or `"PDF"` |
| `tag_ids` | `StringSlice` | notnull |
| `tags` | `[]Tag` | hydrated |
| `file` | `*AssetFile` | hydrated, omitempty |
| `created_by` | string | session user |
| `created_at` / `updated_at` | time | default `current_timestamp` |

- **Status enum:** `pending`, `processing`, `ready`, `partial` (some chunks embedded, some failed), `failed`.
- **Type enum:** `MD`, `PDF`. There is no "imagery"/"text" asset kind — text assets are `type==nil` or `MD`; images are handled by `/api/images` + post attachments, not the asset model.

### AssetFile (`models.AssetFile`, table `asset_files`)

One-to-one with Asset (`UNIQUE(asset_id)`), for file-backed (PDF) assets:
`id`, `asset_id`, `original_name`, `mime_type`, `size_bytes` (int64), `s3_key`
(e.g. `assets/<assetID>/original.pdf`), `thumbnail_s3_key` (`*string`,
`assets/<assetID>/thumbnail.png`), `page_count` (`*int`), timestamps. Transient
`thumbnail_url` (`*string`, omitempty) is filled from `ThumbnailS3Key` via `PublicURL`.

### AssetChunk (`models.AssetChunk`, table `assets_chunks`)

One chunk + embedding. Not exposed via the asset API; populated by async flows.
`id` (`<assetID>:<chunkIndex>`), `asset_id`, `chunk_index`, `page_start`/`page_end`
(`*int`, PDFs), `content`, `token_count`, `embedding` (`[]byte`, not serialized),
`model`, `created_at`.

### PostAttachment (`models.PostAttachment`, table `post_attachments`)

An image or PDF bound to one post; `UNIQUE(post_id, position)`.
`id`, `post_id`, `position` (int), `mime_type`, `size_bytes` (int64), `width`/`height`
(int, images; 0 for PDFs), `is_animated` (bool), `page_count` (int, PDFs; 0 for images),
`checksum_sha256`, `s3_key` (`post-attachments/<postID>/<id><ext>`), `thumbnail_s3_key`
(omitempty; PDFs, `...<id>.thumb.png`), `created_by`, `created_at`. Transient
`presigned_url` / `thumbnail_url` (omitempty) hydrated at response time.

> Asset thumbnails use **`PublicURL`**; post-attachment URLs use short-lived **presigned** GET URLs (attachments are treated as private).

---

## 2. Content-bank asset endpoints — `/api/content-bank/assets`

### `GET /` — List
Returns all assets ordered by `created_at ASC`, tags + file hydrated, `file.thumbnail_url`
decorated when storage configured. → `200 []Asset`.

### `POST /` — Create
Body `createAssetRequest` (JSON): `title` (`required`), `content` (`required`), `tag_ids` (optional).
Creates asset `status="pending"`, `type=nil`, then fires `go h.onSave(id, title, content)`
(async fire-and-forget text embedding). → `201 Asset`. No S3 write.

### `POST /upload` — Upload Markdown or PDF (multipart)
- Form field **`files`** (one or more). Missing/empty → `400 "no files provided (use field 'files')"`. Wrong content type → `400 "expected multipart/form-data"`.
- Per-extension routing (`detectUploadKind`): `.md` → Markdown (**max 10 MB**); `.pdf` → PDF (**max 50 MB**); other → that file fails with `"only .md and .pdf files are accepted"`.
- **Per-file independent processing** — one failure doesn't block others. Always returns **`201`** `{"results":[{filename, asset_id?, status:"created"|"failed", error?, asset?}]}`.
- **Markdown path** (`processMarkdownUpload`): title = filename minus ext; content = raw bytes; asset `status="pending"`, `type="MD"`; fires `go h.onSave(...)`. No S3 write.
- **PDF path** (`processPDFUpload`): validates magic bytes (first 4 == `%PDF`) → else `"file is not a valid PDF"`; title = filename minus ext; content `"[]"`; asset `status="pending"`, `type="PDF"`; calls `h.onPDF(ProcessPDFInput{...})` which runs **async**. S3 writes happen inside the async flow.

#### Async PDF pipeline (`flows.processPDF`), updating asset status:
1. `processing`.
2. Upload original → `assets/<assetID>/original.pdf` (skipped if storage disabled; upload error → `failed`).
3. Extract text per page (ledongthuc primary, pdftotext fallback; error → `failed`).
4. Page-aware chunk (`pdf.ChunkPages`); embed word-bearing chunks → `AssetChunk` (`<id>:<i>`, page range, token count, vector, model); upsert.
5. Render first-page thumbnail @ **96 DPI** → `assets/<assetID>/thumbnail.png` (failures logged, non-fatal).
6. Upsert `asset_files` row.
7. Final status: no chunks → `ready`; some embeds failed but ≥1 succeeded → `partial`; chunks expected but none stored → `failed`; else `ready`.

### `GET /:id` — Get
→ `200 Asset` (tags+file hydrated, thumbnail decorated); not found → `404 "asset not found"`.

### `PUT /:id` — Update
Body `updateAssetRequest`: same as create. Re-embeds via `go h.onSave(...)` **only when
`title` or `content` changed** (tag-only edits don't re-embed). Sets `updated_at`. → `200`.

### `DELETE /:id` — Delete
Collects S3 keys from the `asset_files` row first (DB cascade drops it), deletes the asset
row (`404` if nothing deleted), then **best-effort** S3 deletes (failures swallowed). → `204`.

---

## 3. Image upload — `POST /api/images` (multipart)

Inline editor images. Standalone route.
- Form field **`file`** (single). Missing → `400 "file is required"`. **Max 10 MB** → over → `400`.
- **Server-sniffed MIME** (`http.DetectContentType` on first 512 bytes; client Content-Type ignored): `image/jpeg`→`.jpg`, `image/png`→`.png`, `image/webp`→`.webp`, `image/gif`→`.gif`. Other → `415 "unsupported media type: <mime>"`.
- Storage required → nil → `503 "storage not configured"`. Key `uuid + ext` at bucket root.
- → **`200`** `{"status":"success","data":{"url":"<public URL>"}}`. Codes: `400/401/415/503`.

---

## 4. Post attachments — `/api/posts/:post_id/attachments` (`auth` at group level)

Every route loads the parent post first (`404 "post not found"`).

- **Mutation freeze:** Upload/Reorder/Delete reject posts with status `published` → `409 "post is in a terminal publishing state and its attachments are immutable"`. (Only `published` freezes; other statuses are mutable.)
- **Presigned URL hydration:** when storage configured, `presigned_url` (from `s3_key`) and `thumbnail_url` (from `thumbnail_s3_key`) filled with signed GET URLs, TTL `PresignedURLTTL = 15m`. Presign errors swallowed (metadata still returned).
- **Soft platform validation:** responses include `platform_validation` (`[]platforms.ValidationError`) — warnings only, never blocking. List responses carry post-level + per-attachment validation.

### `GET /` — List
Ordered by `position ASC`. → `200` `{"attachments":[{...PostAttachment, "platform_validation":[...]}], "platform_validation":[...]}`.

### `GET /:id` — Get
`404` if missing or `PostID` mismatch (cross-post access rejected as not-found). → `200` `attachmentResponse` (flattened `PostAttachment` + `platform_validation`).

### `POST /` — Upload (multipart)
- Form field **`file`** (single). Missing → `400 "file is required"`; empty → `400 "file is empty"`.
- Storage required (`503`); post must exist (`404`) and not be `published` (`409`).
- **Size caps:** hard upper bound **100 MB** (`"file exceeds upload limit of 100 MB"`); images **50 MB**; PDFs **100 MB**.
- **Type detection:** sniff first 512 bytes (`http.DetectContentType`). `application/pdf` → PDF path, else image path.
  - **PDF** (`pdfprobe.Probe`): verifies `%PDF-` magic, counts pages, SHA-256. `ErrUnsupportedMIME`→`415`; other → `400`. Sets `mime_type=application/pdf`, `size_bytes`, `page_count`, `checksum_sha256`; ext `.pdf`.
  - **Image** (`imageprobe.Probe`): sniffs MIME, decodes dimensions, SHA-256, animated-GIF flag. Allowed: `image/jpeg`→`.jpg`, `image/png`→`.png`, `image/webp`→`.webp`, `image/gif`→`.gif`. Unsupported → `415`; other → `400`.
- **Key:** `post-attachments/<postID>/<attachmentID><ext>`.
- **PDF thumbnail (best-effort):** first-page PNG @ 96 DPI, 30s timeout → `post-attachments/<postID>/<attachmentID>.thumb.png`. Failures never abort.
- **Ordering:** `CreateAtNextPosition` atomically assigns `position = COALESCE(MAX(position)+1, 0)` (`RETURNING position`) — race-free under concurrent uploads (SQLite serializes writers; `UNIQUE(post_id, position)` never tripped).
- **Rollback:** if the DB insert fails after upload, the orphaned object(s) are deleted.
- → **`201`** `attachmentResponse`. Codes: `400/401/404/409/415/503`.

### `PATCH /:id` — Reorder
Body `reorderRequest`: `position` (`*int`, `required` — so `0` is accepted; missing fails). `*position < 0` → `400 "position must be non-negative"`. **Last-write-wins**, plain `SET position`, no sibling re-packing. → `200 attachmentResponse`.

### `DELETE /:id` — Delete
**S3-first:** deletes `s3_key` then `thumbnail_s3_key`. Any S3 delete failure → row **retained** + `502` (`"failed to delete object from storage; please retry"`). Only after S3 succeeds is the row deleted (`404` if 0 rows). → `204`. Codes: `401/404/409/502`.

---

## 5. Object storage (`src/storage/storage.go`)

`storage.Storage` interface over AWS SDK v2 S3, **path-style** addressing (`UsePathStyle: true`, required for R2/DO Spaces):
- `Upload(ctx, key, r, size, contentType) (publicURL, err)` — `PutObject`.
- `Delete(ctx, key)` — `DeleteObject` (nil even if absent).
- `PublicURL(key)` → `<publicURL base>/<key>`.
- `PresignedGetURL(ctx, key, ttl)` — short-lived signed GET (post-attachments, publish-time media handoff).

### Config — see [README §3](./README.md#3-configuration--environment-variables) (`STORAGE_*`).

### Disabled storage
`storage.New(cfg)` returns `(nil, nil)` when `STORAGE_ENDPOINT == ""`. With nil `Storage`:
- `POST /api/images` and attachment upload → `503 "storage not configured"`.
- Asset list/get/update skip thumbnail decoration; asset delete skips S3 cleanup (row still removed).
- Attachment list/get/reorder work but produce no presigned URLs.
- The async PDF flow still extracts/chunks/embeds but skips the original-PDF + thumbnail uploads.

### Key conventions
- Inline images: `<uuid><ext>` (bucket root, `PublicURL`).
- Asset PDFs: `assets/<assetID>/original.pdf`, thumbnail `assets/<assetID>/thumbnail.png` (`PublicURL`).
- Post attachments: `post-attachments/<postID>/<attachmentID><ext>`, thumbnail `...<attachmentID>.thumb.png` (presigned).

---

## 6. Probes

- **`imageprobe.Probe(r, limit)`**: reads up to `limit+1` bytes (oversized → `"file exceeds limit"`), rejects empty, sniffs MIME (rejects non-`AllowedMIMEs` → `ErrUnsupportedMIME`), decodes `Width`/`Height` (`image.DecodeConfig`; WebP via `x/image/webp`), flags animated GIFs (`gif.DecodeAll`, `frameCount > 1`), SHA-256. Returns buffered bytes. `Result`: `MIME`, `Extension`, `Width`, `Height`, `IsAnimated`, `SHA256`, `Size`.
- **`pdfprobe.Probe(r, limit)`**: same size/empty guards, verifies `%PDF-` prefix (→ `ErrUnsupportedMIME "not a PDF"`), counts pages (`pdf.PageCount`), SHA-256. `MIME="application/pdf"`, `Extension=".pdf"`. `Result`: `MIME`, `Extension`, `PageCount`, `SHA256`, `Size`.

---

## 7. Repository notes

- **`AssetRepository`**: `List` (created_at ASC, hydrates tags+files), `GetByID`, `Update` (`WherePK`), `UpdateStatus` (status only, used by async flow), `Delete` (bool).
- **`AssetFileRepository`**: `Upsert` (`ON CONFLICT (asset_id) DO UPDATE`), `ListByAssetIDs` (batched hydration).
- **`PostAttachmentRepository`**: `ListByPostID` (position ASC), `CreateAtNextPosition` (atomic), `ListS3KeysByPostID` (cascade cleanup), `UpdatePosition`, `Delete` (bool).
