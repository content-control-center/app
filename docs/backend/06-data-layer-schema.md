# Data Layer: DB Setup, Full Schema, IDs, Column Types, Repositories

Covers `src/database/*`, `src/database/migrations/*.up.sql` (current schema after all ~50
migrations), `src/models/{id,password,types,session,user,setting,secret}.go`, and
`src/repository/*`.

Go + [Bun ORM](https://bun.uptrace.dev/) over SQLite (pure-Go `ncruces/go-sqlite3`). Single
binary, single SQLite file. Schema below is reconstructed from all migrations, accounting
for renames/rebuilds (`pieces`→`assets`, `campaign_types`→`campaigns_types`, post status
changes, Zernio fields).

---

## 1. Database setup

### DSN & driver
- Driver `ncruces/go-sqlite3/driver`, registered as `sqlite3`; `sql.Open("sqlite3", dsn)`. (A `sqlite-vec` extension is mentioned in a comment but **not wired** — embeddings are plain `BLOB`s.)
- Default DSN: `file:data/app.db?cache=shared&_pragma=journal_mode(WAL)` (file at `data/app.db` local, `/data/app.db` in container). `cache=shared`; WAL via `_pragma`. **`busy_timeout` and `foreign_keys` are NOT set in the default DSN** — FK enforcement is not forced globally (individual rebuild migrations toggle `PRAGMA foreign_keys` locally).

### Connection pool (single connection)
`database.New` sets `SetMaxOpenConns(1)` + `SetMaxIdleConns(1)`, wraps in
`bun.NewDB(sqldb, sqlitedialect.New())`, adds `bundebug` when `debug`, then `Ping`s.
(Tests override the single-conn cap.)

### Migrations (Bun migrator, embedded, idempotent)
`migrate.go` `//go:embed migrations/*.sql`. `migrate.NewMigrations()` +
`migrations.Discover()` find `*.up.sql`/`*.down.sql` pairs by filename
(`<timestamp>_<name>.{up,down}.sql`), ordered by numeric prefix. `migrator.Init` creates
`bun_migrations`/`bun_migration_locks`; `migrator.Migrate` applies all pending as one group.
Runs on **every startup** (and at the start of seeding) — new files picked up automatically.

---

## 2. Full schema (current state after all migrations)

> Bun bookkeeping tables `bun_migrations`/`bun_migration_locks` also exist.

### `users`
`id` TEXT PK (Sqid) · `name` TEXT NOT NULL · `email` TEXT NOT NULL **UNIQUE** ·
`password_hash` TEXT NOT NULL DEFAULT `''` (argon2id PHC) · `created_at`/`updated_at`
DATETIME NOT NULL DEFAULT `CURRENT_TIMESTAMP`.

### `sessions`
`id` TEXT PK (random 32-byte token) · `user_id` TEXT NOT NULL **FK→users(id) ON DELETE
CASCADE** · `expires_at` DATETIME NOT NULL · `created_at` DATETIME NOT NULL DEFAULT now.

### `settings`
`key` TEXT PK · `value` TEXT NOT NULL. **Seeded:** `('setup_complete','false')`.

### `assets` (was `pieces`; renamed `20260415000001`, rebuilt `20260416000001`)
The rebuild widened CHECK constraints and **dropped the `created_by` FK** (plain TEXT now).
`id` TEXT PK · `title` TEXT NOT NULL · `content` TEXT NOT NULL (BlockNote/Markdown) ·
`status` TEXT NOT NULL DEFAULT `'ready'` **CHECK IN (pending,processing,ready,partial,failed)** ·
`type` TEXT NULL **CHECK (type IS NULL OR type IN ('MD','PDF'))** · `tag_ids` TEXT NOT NULL
(JSON array) · `created_by` TEXT NOT NULL (no FK after rebuild) · `created_at`/`updated_at`.

> The old `pieces_embeddings` (1:1 BLOB) was renamed to `assets_embeddings` then **dropped** by `20260415000003` after migrating into `assets_chunks`.

### `asset_files` (`20260416000001`)
1:1 metadata for file-backed (PDF) assets.
`id` TEXT PK · `asset_id` TEXT NOT NULL **UNIQUE** **FK→assets(id) ON DELETE CASCADE** ·
`original_name`, `mime_type` TEXT NOT NULL · `size_bytes` INTEGER NOT NULL · `s3_key` TEXT
NOT NULL · `thumbnail_s3_key` TEXT NULL · `page_count` INTEGER NULL · timestamps.
**Index:** `idx_asset_files_asset_id (asset_id)`.

### `assets_chunks` (`20260415000003`)
`id` TEXT PK (`<asset_id>:<idx>`) · `asset_id` TEXT NOT NULL **FK→assets(id) ON DELETE
CASCADE** · `chunk_index` INTEGER NOT NULL · `page_start`/`page_end` INTEGER NULL ·
`content` TEXT NOT NULL DEFAULT `''` · `token_count` INTEGER NOT NULL DEFAULT 0 ·
`embedding` BLOB NULL (raw vector) · `model` TEXT NULL · `created_at`.
**Constraints:** `UNIQUE(asset_id, chunk_index)`; `idx_assets_chunks_asset_id (asset_id)`.

### `campaigns` (rebuilt `20240114000001`, `20260414000001`; cols renamed `20260415000002`)
`id` TEXT PK · `name` TEXT NOT NULL · `description`/`target_persona`/`key_messages`/
`tone_guidelines` TEXT NOT NULL DEFAULT `''` · `use_assets` BOOLEAN NOT NULL DEFAULT FALSE
(was `use_pieces`) · `asset_ids` TEXT NOT NULL DEFAULT `'[]'` (was `pieces_ids`) ·
`target_platforms` TEXT NOT NULL DEFAULT `'[]'` (JSON `[{id, post_types:[]}]`) ·
`campaign_type_id` TEXT NOT NULL **FK→campaigns_types(id)** (no ON DELETE → RESTRICT) ·
`status` TEXT NOT NULL DEFAULT `'draft'` · `start_date`/`end_date` DATETIME NULL ·
`estimated_post_count` INTEGER NULL · `budget` REAL NULL · `currency`/`language` TEXT NOT
NULL DEFAULT `''` · `tag_ids` TEXT NOT NULL DEFAULT `'[]'` · `created_by` TEXT NOT NULL
**FK→users(id) ON DELETE CASCADE** · timestamps.

> `target_platform_ids` was reshaped into the JSON `target_platforms` array and dropped (`20240114000001`).

### `campaigns_types` (created as `campaign_types` `20260414000001`, renamed `20260414000003`)
`id` TEXT PK (deterministic Sqid) · `name` TEXT NOT NULL **UNIQUE** · `label`/`description`
TEXT NOT NULL DEFAULT `''` · `is_system` BOOLEAN NOT NULL DEFAULT FALSE (`20260414000002`) ·
timestamps.
**Seeded (`is_system=TRUE`):** `Uk`=awareness, `gb`=engagement, `Ef`=conversion,
`Vq`=retention, `uw`=evergreen (`20260425000002`).

### `campaigns_types_phases` (created as `campaign_type_phases`, renamed `20260414000003`)
`id` TEXT PK · `campaign_type_id` TEXT NOT NULL **FK→campaigns_types(id) ON DELETE CASCADE** ·
`name` TEXT NOT NULL · `purpose` TEXT NOT NULL DEFAULT `''` · `sequence` INTEGER NOT NULL ·
timestamps. **Seeded** phases for each system type.

### `platforms`
`id` TEXT PK (Sqid, migrated from slugs `20240113`/`20240115`) · `name` TEXT NOT NULL ·
`post_types` TEXT NOT NULL DEFAULT `'{}'` (JSON slug→label) · `cadence` TEXT NOT NULL DEFAULT
`''` (`20240112000001`) · `constraints` TEXT NOT NULL DEFAULT `''` · `image_constraints` TEXT
NOT NULL DEFAULT `'{}'` (`20260507000002`) · `pdf_constraints` TEXT NOT NULL DEFAULT `'{}'`
(`20260508000002`) · timestamps.

**Seeded (6 platforms, current Sqid IDs):**

| Sqid id | name | image (size/formats/animated/max) | pdf |
|---|---|---|---|
| `AXqWG7U2qnpt` | LinkedIn | 10 MiB / jpeg,png,gif / no / 9 | 100 MB, pdf, 300 pages, 1/post |
| `8S8bWQTG6qD` | YouTube | 16 MiB / jpeg,png,gif / yes / 1 | `{}` |
| `zBU1zqVICGfk` | Facebook | 30 MiB / jpeg,png,gif,webp / yes / 10 | `{}` |
| `81mUCmc2xsKd` | X (Twitter) | 5 MiB / jpeg,png,webp,gif / yes / 4 | `{}` |
| `pQ4yxT3SuE57` | Threads | 8 MiB / jpeg,png,gif / yes / 20 | `{}` |
| `rzgpTkARLH0L` | Instagram | 8 MiB / jpeg,png / no / 20 | `{}` |

### `posts` (created `20240110000001`, rebuilt `20260425000001`; cols added later)
The rebuild relaxed `platform_id`/`platform_post_type` to nullable for drafts and flipped
the platform FK from CASCADE to **SET NULL**.
`id` TEXT PK · `campaign_id` TEXT NOT NULL **FK→campaigns(id) ON DELETE CASCADE** ·
`platform_id` TEXT NULL **FK→platforms(id) ON DELETE SET NULL** · `platform_post_type` TEXT
NULL · `title` TEXT NOT NULL · `content` TEXT NOT NULL DEFAULT `''` · `media_urls` TEXT NOT
NULL DEFAULT `'[]'` · `scheduled_at`/`published_at` DATETIME NULL · `status` TEXT NOT NULL
DEFAULT `'draft'` · `cta_type` TEXT NOT NULL DEFAULT `'none'` · `cta_url`/
`target_audience_notes` TEXT NOT NULL DEFAULT `''` · `used_asset_ids` TEXT NOT NULL DEFAULT
`'[]'` (was `used_pieces_ids`) · `campaign_type_phase_id` TEXT NULL
**FK→campaigns_types_phases(id)** (`20260414000004`) · `zernio_post_id` TEXT NULL
(`20260509000002`) · `zernio_status`/`published_results`/`failure_reason` TEXT NOT NULL
DEFAULT `''` · `created_by` TEXT NOT NULL **FK→users(id) ON DELETE CASCADE** · timestamps.
**Index:** `idx_posts_zernio_post_id` — partial UNIQUE on `(zernio_post_id) WHERE
zernio_post_id IS NOT NULL` (prevents double-submit; allows many NULLs).

**Status values** (after `20260416000002` mapped `in_review`/`approved` → `ready_for_publish`;
**no DB-level CHECK** — enforced in code via `models.ValidPostTransitions`): `draft`,
`ready_for_publish`, `scheduled`, `scheduled_for_manual_publishing`, `failed`, `published`,
`not_published`.

### `post_versions` (`20260417000001`)
`id` TEXT PK · `post_id` TEXT NOT NULL **FK→posts(id) ON DELETE CASCADE** · `version_number`
INTEGER NOT NULL · `content` TEXT NOT NULL (renamed from `description` `20260424000001`) ·
`note` TEXT NOT NULL DEFAULT `''` · `creator` TEXT NOT NULL **CHECK IN (assistant,user)** ·
`created_at`. **Indexes:** `idx_post_versions_post_id`; **UNIQUE** `idx_post_versions_post_version (post_id, version_number)`.

### `post_assistant_messages` (`20260417000002`)
`id` TEXT PK · `post_id` TEXT NOT NULL **FK→posts(id) ON DELETE CASCADE** · `role` TEXT NOT
NULL **CHECK IN (user,model)** · `content` TEXT NOT NULL · `created_at`.
**Index:** `idx_post_assistant_messages_post_id`.

### `post_attachments` (`20260507000001`; PDF cols `20260508000001`)
`id` TEXT PK · `post_id` TEXT NOT NULL **FK→posts(id) ON DELETE CASCADE** · `position`
INTEGER NOT NULL · `mime_type` TEXT NOT NULL · `size_bytes` INTEGER NOT NULL · `width`/
`height` INTEGER NOT NULL DEFAULT 0 · `is_animated` INTEGER NOT NULL DEFAULT 0 ·
`page_count` INTEGER NOT NULL DEFAULT 0 · `checksum_sha256` TEXT NOT NULL · `s3_key` TEXT NOT
NULL · `thumbnail_s3_key` TEXT NULL · `created_by` TEXT NOT NULL **FK→users(id) ON DELETE
CASCADE** · `created_at`. **Constraints:** `UNIQUE(post_id, position)`;
`idx_post_attachments_post_id`.

### `post_logs` (`20260509000001`)
`id` TEXT PK (writer-supplied) · `post_id` TEXT NOT NULL **FK→posts(id) ON DELETE CASCADE** ·
`event_timestamp` DATETIME NOT NULL DEFAULT now · `event_type` TEXT NOT NULL · `actor` TEXT
NOT NULL · `from_status`/`to_status` TEXT NULL · `payload` TEXT NOT NULL DEFAULT `'{}'`
(sanitized JSON ≤64 KB, code-enforced) · `summary` TEXT NOT NULL DEFAULT `''`.
**Indexes:** `idx_post_logs_post_id`, `idx_post_logs_event_timestamp`, `idx_post_logs_event_type`.

### `tags`
`id` TEXT PK · `name` TEXT NOT NULL · `color` TEXT NOT NULL DEFAULT `''` · `created_by` TEXT
NOT NULL **FK→users(id) ON DELETE CASCADE** · timestamps.

### `social_accounts` (`20260425000003`)
Local view of Zernio accounts; PK = Zernio accountId; soft-delete via `deleted_at`.
`id` TEXT PK · `platform`/`profile_id` TEXT NOT NULL · `username`/`display_name`/
`avatar_url` TEXT NOT NULL DEFAULT `''` · `is_active` BOOLEAN NOT NULL DEFAULT 1 · `raw_json`
TEXT NOT NULL DEFAULT `'{}'` · `connected_at`/`last_synced_at` TIMESTAMP NOT NULL ·
`deleted_at` TIMESTAMP NULL. **Partial indexes (active only):**
`idx_social_accounts_platform_active (platform) WHERE deleted_at IS NULL`,
`idx_social_accounts_profile_active (profile_id) WHERE deleted_at IS NULL`.

### `secret` (`20260429000001`)
Envelope-encrypted API keys. **Only table with an integer rowid PK.**
`id` INTEGER PK AUTOINCREMENT · `name` TEXT NOT NULL **UNIQUE** · `ciphertext`/`nonce`/
`wrapped_dek`/`dek_nonce` BLOB NOT NULL · `algorithm` TEXT NOT NULL DEFAULT `'AES-256-GCM'` ·
`kek_version` INTEGER NOT NULL DEFAULT 1 · `created_at`/`updated_at` DATETIME NOT NULL.

### `auto_publish_allowlist` (`20260501000001`)
`platform_id` TEXT PK — Zernio **wire identifier** (e.g. `"linkedin"`), **not** a
`platforms.id` Sqid; intentionally **not FK'd** (validated in repo against
`zernio.LookupSupportedPlatform`) · `created_at` DATETIME NOT NULL DEFAULT now. Empty table
= auto-publish disabled everywhere (opt-in default).

---

## 3. ID & password strategy

### IDs (Sqids) — `src/models/id.go`
`NewID()` reads a crypto-random `uint64` and encodes via `sqids-go` into a short URL-safe
string (package singleton). Seed rows use deterministic Sqids hand-encoded in migrations
(campaign types `Encode([1..5])`, phases `Encode([10..52])`). Exceptions: `platforms.id`
fixed Sqids; `social_accounts.id` = Zernio accountId; `secret.id` integer rowid;
`sessions.id` random 32-byte token (`NewSessionToken`, base64 RawURLEncoding).

### Passwords — `src/models/password.go`
**argon2id** (not bcrypt — the docstring is historical). `HashPassword` → PHC string
`$argon2id$v=19$m=...,t=...,p=...$<salt>$<hash>`. Production params (`DefaultArgon2Params` =
`ProductionArgon2Params`): 64 MiB memory, 3 iters, parallelism 2, 16-byte salt, 32-byte key.
`FastTestArgon2Params` (8 KiB/1/1) for tests. `VerifyPassword` parses params from the stored
hash (verifies after cost changes) and compares with `subtle.ConstantTimeCompare`.

---

## 4. Custom column types — `src/models/{types,platform}.go`

All implement `driver.Valuer` (→ JSON string) + `sql.Scanner` (string/`[]byte`/nil),
persisting into SQLite **TEXT**:

| Type | Go | Serializes as | Used by |
|---|---|---|---|
| `StringSlice` | `[]string` | JSON array (nil → `StringSlice{}`) | `assets.tag_ids`, `campaigns.asset_ids`/`tag_ids`, `posts.media_urls`/`used_asset_ids` |
| `CampaignPlatforms` | `[]{ID, PostTypes []string}` | JSON `[{"id","post_types":[]}]` | `campaigns.target_platforms` |
| `PostTypeMap` | `map[string]string` | JSON object | `platforms.post_types` |
| `ImageConstraints` | struct | JSON object (empty → zero; `IsZero()`) | `platforms.image_constraints` |
| `PDFConstraints` | struct | JSON object (empty → zero; `IsZero()`) | `platforms.pdf_constraints` |

Plain BLOBs (`assets_chunks.embedding`, `secret.*`) are `[]byte` directly.
`Post.PlatformID`/`PlatformPostType` use Bun `nullzero` → empty string persists as SQL NULL
(needed for the nullable platform FK on drafts).

---

## 5. Repository catalog

All in `src/repository`, depend on `*bun.DB`, exposed as interfaces with `New…Repository(...)`
constructors, wired in `src/server/server.go`. Common CRUD shape; `Delete` returns
`(bool, error)`. Hydration helpers in `src/repository/hydrate.go` (`HydrateTags`,
`collectIDs*`, generic `fetchByIDs`).

| Repository | Model / table | Notable methods |
|---|---|---|
| `UserRepository` | `User`/`users` | `GetByEmail` (login) |
| `SessionRepository` | `Session`/`sessions` | `Create`/`GetByID`/`Delete` |
| `SettingRepository` | `Setting`/`settings` | `GetByKey`, **`Upsert`** (`ON CONFLICT(key)`) |
| `SecretRepository` | `Secret`/`secret` | `Get(name)`, **`Upsert`** (`ON CONFLICT(name)`), `Delete(name)` |
| `TagRepository` | `Tag`/`tags` | **`GetByIDs → map`** (hydration) |
| `AssetRepository` | `Asset`/`assets` | **`UpdateStatus`**; composes Tag+AssetFile repos; hydrates `Tags`/`File` |
| `AssetFileRepository` | `AssetFile`/`asset_files` | **`Upsert`**, **`ListByAssetIDs → map`** |
| `AssetChunksRepository` | `AssetChunk`/`assets_chunks` | **`UpsertChunks`** (delete-then-insert in `RunInTx`, **`withRetry`** 3× exp backoff on transient SQLite errors), **`GetAllEmbedded`** (similarity search), `GetByIDs` |
| `CampaignRepository` | `Campaign`/`campaigns` | hydrates `Platforms`/`CampaignType`/`Tags` |
| `CampaignTypeRepository` | type + phase | **`GetByIDs → map`**, phase methods `AddPhase`/`GetPhaseByID`/`UpdatePhase`/`DeletePhase` |
| `PlatformRepository` | `Platform`/`platforms` | CRUD |
| `PostRepository` | `Post`/`posts` | **`ListByCampaign`**, **`CreateBatch`** (`RunInTx`), **`ListStuckScheduled(cutoff, limit)`** (reconcile), **`UpdateStatusAndReason`**; `hydrateRelations` batch-loads campaign/platform/used-assets/phase |
| `PostVersionRepository` | `PostVersion`/`post_versions` | **`GetLatestByPostID`**, **`CountByPostID`** |
| `PostAssistantMessageRepository` | `post_assistant_messages` | **`ListRecentByPostID(id, limit)`** |
| `PostAttachmentRepository` | `post_attachments` | **`CreateAtNextPosition`**, **`ListS3KeysByPostID`**, **`UpdatePosition`** |
| `PostLogRepository` | `PostLog`/`post_logs` | **`Append`**, **`AppendTx(tx, …)`** (joins outer tx), **`ListFiltered`**, **`DeleteOlderThan → int64`** (no sanitization — writers pre-sanitize) |
| `SocialAccountRepository` | `social_accounts` | `ListAll`/`ListActive`, **`ApplyPlan(upserts, softDeleteIDs, now)`** (single `RunInTx`: upsert/revive + bulk soft-delete) |
| `AutoPublishAllowlistRepository` | `auto_publish_allowlist` | `List → []string`, **`Set`** (replace whole list), **`Contains`** |
