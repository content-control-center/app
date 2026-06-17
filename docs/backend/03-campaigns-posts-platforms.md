# Campaigns, Campaign Types, Posts, Tags, Platforms

Covers `src/handlers/{campaigns,campaign_types,posts,tags,platforms}.go`, the related
models, and `src/platforms/{constraints,post_types,validate}.go`.

All endpoints require auth (`401` otherwise). Bodies are JSON, validated via
`validate.Struct` (failure → `400` flattened message). Path IDs are Sqids.

---

## 1. Campaigns — `/api/campaigns`

### Model `models.Campaign` (table `campaigns`)
`id`, `name`, `description`, `target_persona`, `key_messages`, `tone_guidelines`,
`use_assets` (bool), `asset_ids` (`StringSlice`), `target_platforms`
(`CampaignPlatforms` = `[]{id, post_types[]}`), `campaign_type_id` (FK),
`start_date`/`end_date` (`*time.Time`), `estimated_post_count` (`*int`), `language`,
`status` (`CampaignStatus`), `budget` (`*float64`), `currency`, `tag_ids` (`StringSlice`),
hydrated `tags`/`platforms`/`campaign_type`, `created_by`, timestamps.

**`CampaignStatus` enum:** `draft`, `scheduled`, `active`, `paused`, `completed`,
`archived`. **No state machine** — any valid status can be set on create/update.

### Request `campaignRequest` (Create + Update)
`name` (`required`), `campaign_type_id` (`required`), plus all other fields optional.
`status` defaults to `draft` when empty. Empty `asset_ids`/`target_platforms`/`tag_ids`
normalized to empty JSON arrays.

| Method + Path | Success | Notes |
|---|---|---|
| `GET /` | `200 []Campaign` | All, by creation date |
| `POST /` | `201 Campaign` | Validates status (`400 "invalid status"`) + `campaign_type_id` exists (`400 "invalid campaign_type_id"`); sets `created_by` |
| `GET /:id` | `200 Campaign` | `404` if absent |
| `PUT /:id` | `200 Campaign` | Same validation; overwrites all mutable fields; preserves `created_at`/`created_by` |
| `DELETE /:id` | `204` | `404 "campaign not found"` if nothing deleted |
| `POST /:id/generate-draft` | `200` SSE | AI; see [`05-ai-genkit-flows.md`](./05-ai-genkit-flows.md) |

### `POST /:id/generate-draft` (AI, SSE) — HTTP contract
- `503 "content plan feature is not enabled"` if `generateDraft` nil or `isContentPlanReady()` false (checked **before** the stream opens).
- Loads campaign (`404`). **Ownership gate:** `campaign.CreatedBy != session.UserID` → `403 "forbidden"`.
- Sets `text/event-stream` + `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no`.
- SSE events: intermediate `step` events; `complete` (full `ContentPlanResponse`); `error` `{"message", "code"}` (`ValidationError`→400, `AIError`→502, else 500).

---

## 2. Campaign Types & Phases — `/api/campaign_types`

A campaign type is a named template/classification (`name`, `label`, `description`,
`is_system`, ordered `phases`). A campaign references one via `campaign_type_id`.
**System types** (`is_system=true`, seeded) are immutable: cannot be updated or deleted
(but can be cloned). User-created types are always `is_system=false`.

### Models
- `CampaignType` (table `campaigns_types`): `id`, `name`, `label`, `description`, `is_system`, hydrated `phases`. Timestamps `json:"-"`.
- `CampaignTypePhase` (table `campaigns_types_phases`): `id`, `campaign_type_id`, `name`, `purpose`, `sequence` (int ordering). Timestamps `json:"-"`.

### Requests
- `campaignTypeRequest`: `name` (`required`), `label` (`required`), `description`.
- `cloneCampaignTypeRequest`: `name` (`required`), `label` (`required`).
- `campaignTypePhaseRequest`: `name` (`required`), `purpose`, `sequence` (int, `required` — so `0` is rejected as missing; phases must be non-zero indexed).

| Method + Path | Success | Notes |
|---|---|---|
| `GET /` | `200 []CampaignType` | With phases, by name |
| `POST /` | `201 CampaignType` | New user type, `is_system=false`, no phases |
| `GET /:id` | `200 CampaignType` | `404` if absent |
| `PUT /:id` | `200 CampaignType` | **`403` if `is_system`** (`"system campaign type cannot be modified"`) |
| `DELETE /:id` | `204` | **`403` if `is_system`**; `404` if 0 rows |
| `POST /:id/clone` | `201 CampaignType` | Deep-copy + phases; **system types CAN be cloned**; clone is `is_system=false` |
| `POST /:id/phases` | `201 CampaignTypePhase` | Parent must exist (`404`); **does NOT block system types** |
| `PUT /:id/phases/:phase_id` | `200 CampaignTypePhase` | `404` if phase absent or `CampaignTypeID != :id` |
| `DELETE /:id/phases/:phase_id` | `204` | Same ownership `404`; `404` if 0 rows |

---

## 3. Posts — `/api/posts` (core resource)

The most stateful resource: status state machine, publish-validation gate, auto-publish
routing, audit logging (`PostLog`), AI assistant streaming, versions, Zernio integration.

### Model `models.Post` (table `posts`)
`id`, `campaign_id` (notnull), `platform_id` (`nullzero` → NULL; FK), `platform_post_type`
(`nullzero`), `title`, `content` (notnull, **Markdown**), `media_urls` (`StringSlice`),
`scheduled_at`/`published_at` (`*time.Time`), `status` (`PostStatus`), `zernio_post_id`
(`nullzero`, omitempty; UNIQUE — prevents double-submit), `zernio_status`,
`published_results` (per-platform JSON), `failure_reason`, `cta_type` (`PostCTAType`),
`cta_url`, `target_audience_notes`, `used_asset_ids` (`StringSlice`),
`campaign_type_phase_id` (`*string`), `created_by`, timestamps, hydrated
`campaign`/`platform`/`used_assets`/`campaign_type_phase`.

**`PostCTAType` enum:** `link`, `button`, `none` (empty defaults to `none`).

### Post status state machine

**`PostStatus` values:** `draft`, `ready_for_publish`, `scheduled`,
`scheduled_for_manual_publishing`, `failed`, `published`, `not_published`.

**Allowed transitions** (`models.ValidPostTransitions`, via `PostStatus.CanTransition`; self-transitions always allowed):

| From | → |
|---|---|
| `draft` | `ready_for_publish` |
| `ready_for_publish` | `scheduled`, `scheduled_for_manual_publishing`, `draft` |
| `scheduled` | `failed`, `published`, `ready_for_publish`, `draft` |
| `scheduled_for_manual_publishing` | `published`, `not_published` |
| `failed` | `ready_for_publish` |
| `not_published` | `ready_for_publish`, `scheduled_for_manual_publishing` |
| `published` | *(terminal)* |

- `scheduled → ready_for_publish` / `scheduled → draft` support user cancellation (via `/cancel`, not direct edits).
- `failed → ready_for_publish` is the manual-retry edge; on success an extra `user_retry` log line carries the prior `failure_reason` + `zernio_post_id`.
- A disallowed transition → `400 "invalid status transition from <X> to <Y>"` + `state_transition_blocked` log.
- `scheduled → published/failed` edges exist but in normal operation are driven by background Zernio jobs, not the `PUT` handler.

### Request `postRequest` (Create + Update)
`campaign_id` (`required`); `platform_id` + `platform_post_type` required only when
resolved status ≠ `draft` (`requirePlatformIfNotDraft` → else `400`); `title`, `content`
(Markdown), `media_urls`, `scheduled_at`, `published_at`, `status` (default `draft`),
`cta_type` (default `none`), `cta_url`, `target_audience_notes`, `used_asset_ids`,
`campaign_type_phase_id`.

| Method + Path | Success | Notes |
|---|---|---|
| `GET /` | `200 []Post` | Hydrated, by creation date |
| `POST /` | `201 Post` | Status/CTA validation + create gate. Can land directly in non-draft (only structural gate runs; no transition check) |
| `GET /:id` | `200 Post` | `404` if absent |
| `PUT /:id` | `200 Post` (re-fetched) | State machine + gate + scheduling (below) |
| `DELETE /:id` | `204` | Runs `onBeforeDelete` (attachment S3 cleanup) first; hook error fails DELETE; `404` if 0 rows |
| `POST /:id/assistant` | `200` SSE | AI; see below |
| `GET /:id/messages` | `200 []PostAssistantMessage` | Most recent 50 |
| `GET /:id/versions` | `200 []PostVersion` | |
| `POST /:id/versions` | `201 PostVersion` | Snapshot current content |
| `POST /:id/cancel` | `202` | Cancel a Scheduled post |
| `GET /api/campaigns/:campaign_id/posts` | `200 []Post` | Mounted under campaigns group |

### Publish validation gate
Two entry points sharing `platforms.ValidateForPublish` + `platforms.ValidatePostType` (§5):
- **`validateForCreate`** (Create, status ≠ draft, platform set): loads platform (`400 "platform not found"`), runs structural + post-type rules. Failure → **`422`** `{"error":"post is not ready for publish","platform_validation":<map[platformID][]ValidationError>}`.
- **`validateReadyForPublish`** (Update, only `draft → ready_for_publish` with attachment repo wired): lists attachments, validates against incoming platform + post-type. Failure → `validation_failed` log + **`422`**; pass → `validation_passed` log, update continues.

### Auto-publish routing (`routeAndPersistSchedule`)
On `ready_for_publish → scheduled` when allowlist repo + jobs client + db are wired:
1. Resolve platform Sqid → Zernio platform (`zernio.LookupSupportedBySqid`).
2. If supported and `allowlistRepo.Contains(zernioID)` → target `scheduled`; else → `scheduled_for_manual_publishing`.
3. **Single SQLite tx:** update post to target status, append `allowlist_decision` log, append `state_transition` log, and (only when auto-publish chosen) enqueue `SubmitPostTask{PostID}` joined to the same tx.
4. Returns the routed status, surfaced via `X-Auto-Publish-Decision` response header.

### `POST /:id/cancel`
Body `cancelRequest` (optional): `target` ∈ `"ready_for_publish"` (default) | `"draft"`; invalid → `400`.
- `503 "background job runtime not configured"` if jobs client nil.
- Post must exist (`404`) and be `scheduled` → else `409 "only Scheduled posts can be cancelled (current status: <status>)"`.
- Enqueues `CancelZernioJobTask{PostID, Target, Actor}`; **post stays `scheduled`** until the job confirms with Zernio. Race (already published) → cancel is a no-op, post lands `published` via poll.
- Writes `user_cancel` log. → **`202`** `{"status":"cancellation_enqueued","target":"...","post_id":"..."}`.

### `POST /:id/assistant` (AI, SSE)
Body `assistantRequest`: `instruction` (`required`).
- `503 "post assistant is not available"` if assistant nil or `isAssistantReady` false (before stream opens). `400` on parse/validation.
- SSE events: `explanation_delta`/`content_delta` (`{"delta":"..."}` Markdown), `tool_call`/`tool_result` (asset retrieval), `complete` (final `PostAssistantResponse`, `updatedContent` is Markdown), `error` (`ValidationError`→400, `AIError`→502, else 500). See [`05-ai-genkit-flows.md`](./05-ai-genkit-flows.md).

### Messages & Versions
- `GET /:id/messages` → most recent 50 (`ListRecentByPostID(id, 50)`). `PostAssistantMessage` (table `post_assistant_messages`): `id`, `post_id`, `role`, `content`, `created_at`.
- `GET /:id/versions` → all snapshots. `POST /:id/versions` (`createVersionRequest`: `note`, optional, **not struct-validated**): snapshots **current `post.Content`**, `version_number = latest+1`, `creator="user"`. → `201`. `PostVersion` (table `post_versions`): `id`, `post_id`, `version_number`, `content` (snapshot), `note`, `creator` (`"user"`/`"assistant"`), `created_at`.

### PostLog (audit side effect)
Every meaningful op appends a `PostLog` when wired (best-effort; failures swallowed).
Event types in this handler: `state_transition`, `state_transition_blocked`,
`validation_failed`, `validation_passed`, `allowlist_decision`, `user_retry`,
`user_cancel`. See [`04-publishing-zernio-jobs.md`](./04-publishing-zernio-jobs.md) for the full vocabulary.

---

## 4. Tags — `/api/tags`

Plain CRUD. `models.Tag` (table `tags`): `id`, `name`, `color`; `created_by`/timestamps
are `json:"-"`. Request `tagRequest`: `name` (`required`), `color`.
`GET /` `200 []Tag` · `POST /` `201` (`created_by` from session) · `GET/PUT/DELETE /:id`
(`200`/`200`/`204`, `404` if absent).

---

## 5. Platforms — `/api/platforms`

### Model `models.Platform` (table `platforms`)
`id`, `name`, `post_types` (`PostTypeMap` = slug→label), `cadence`, `constraints`
(free-text), `image_constraints` (`ImageConstraints`), `pdf_constraints`
(`PDFConstraints`), timestamps.
- `ImageConstraints`: `max_file_size_bytes`, `allowed_formats[]`, `animated_gif_supported`, `max_attachments_per_post`. Zero value (`IsZero()`) = no image rules.
- `PDFConstraints`: `max_file_size_bytes`, `allowed_formats[]`, `max_pages`, `max_attachments_per_post`. Zero value = platform doesn't accept PDFs → validator emits `pdf_not_supported`.

### Request `platformRequest`
`name` (`required`), `post_types` (`PostTypeMap`), `cadence`, `constraints`. **Image/PDF
constraints are NOT settable via this body** (seeded/managed elsewhere).

| Method + Path | Success | Notes |
|---|---|---|
| `GET /` | `200 []platformResponse` | Platform + `publishers` enrichment |
| `POST /` | `201 Platform` | Plain model |
| `GET /:id` | `200 platformResponse` | `404` if absent |
| `GET /:id/post-type-rules` | `200 []PostTypeRuleView` | See below |
| `PUT /:id` | `200 Platform` | `404` if absent |
| `DELETE /:id` | `204` | `404` if 0 rows |

### Enriched response (`platformResponse`)
Embeds `*models.Platform` + additive `publishers` array (always present, `[]` when none).
Each `publisherView`: `id`, `name`, `state`, `connected` (≥1 account), `auto_publish_allowed`,
`supported_post_types[]`, `accounts[]` (`accountView`: `id`, `username`, `display_name`,
`avatar_url`, `is_active`, `connected_at`).

`collectPublisherViews`: for each publisher, `PlatformViews(ctx)` once; match to local
platform by `OgenPlatformID` then case-insensitive `Name`; unmatched dropped.
`auto_publish_allowed` from the allowlist (queried once/request); nil repo or query error
degrades to "nothing allowlisted" (false) rather than failing.

### `GET /:id/post-type-rules`
`[]PostTypeRuleView` sorted by slug: `slug`, `label`, `whitelist_only` (true when no
structural rule), `rule` (`*ResolvedPostTypeRule` or null). `ResolvedPostTypeRule`:
`requires_content`, `allowed_kinds[]`, `min_attachments`, `max_attachments` (`*int`, null =
unbounded — for unbounded rules resolved against the platform's per-kind cap when set).

---

## 6. Platform / post-type validation (`src/platforms`)

Two concerns: **attachment constraints** (on the platform row) + **per-post-type
structural rules** (a static table).

### `ValidationError`
`{platform, attachment_id, rule, expected, actual, message}`. Stable `rule` identifiers:
`max_file_size_bytes`, `allowed_formats`, `animated_gif_supported`,
`max_attachments_per_post`, `max_pages`, `attachment_kind_mix`, `pdf_not_supported`,
`post_type_unknown`, `requires_content`, `min_attachments`, `max_attachments`,
`attachment_kind`.

**Attachment kinds** (`AttachmentKind(mime)`): `image` (jpeg/png/webp/gif), `pdf`, `video`
(any `video/*`), or `""` (unclassified).

### Attachment constraint validation (`validate.go`)
- `ValidateAttachment(att, platform)`: nil platform → nil. Image (skipped if `ImageConstraints.IsZero()`): size, format whitelist, animated-GIF. PDF: if `PDFConstraints.IsZero()` → single `pdf_not_supported`; else size, format, page cap.
- `ValidatePostAttachments`: per-attachment + post-level — `attachment_kind_mix` (images + PDFs together), per-kind count caps. Image-only zero-rule platforms stay silent.
- `ValidateForPublish(atts, platforms)`: publish-time hard check → `map[platformID][]ValidationError`. Platforms with **neither** image nor PDF rules are absent from the map; platforms with rules always get an entry (empty = pass). Any non-empty list = failure.

### Per-post-type structural rules (`post_types.go`)
`PostTypeRule`: `RequiresContent`, `AllowedKinds[]` (nil = no restriction),
`MinAttachments`, `MaxAttachments` (`-1` = unbounded, defers to platform per-kind cap).

| Slug | RequiresContent | AllowedKinds | Min | Max |
|---|---|---|---|---|
| `text-post` | — | — | 0 | 0 |
| `image-post` | — | image | 1 | -1 |
| `carousel` | — | image | 2 | -1 |
| `video` / `reel` / `short` | — | video | 1 | 1 |
| `poll` | yes | — | 0 | 0 |
| `thread` / `article` / `long-form-post` / `newsletter` | yes | — | 0 | -1 |
| `story` | — | image, video | 1 | 1 |

A slug in `platform.post_types` but **absent** here is **whitelist-only** (passes;
platform handles it server-side, e.g. `live-video`, `event`).

`ValidatePostType(post, platform, atts)`: nil platform/post or empty type → nil. Type not
in `platform.post_types` → one `post_type_unknown` (expected = sorted slugs). Whitelist-only
→ nil. Else checks `requires_content` (trimmed), `min_attachments`, `max_attachments`
(skipped if -1), then per-attachment `allowed_kinds` (one `attachment_kind` error per
offender, carrying `attachment_id`). These fold into the same per-platform
`platform_validation` map as the publish gate (unified `422` body).
