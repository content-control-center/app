# AI Subsystem: Genkit Runtimes, Content-Plan & Post-Assistant Flows, Embedding/RAG

Covers `src/server/{genkit_runtime,content_plan,post_assistant,embedding,ssefix}.go`,
`src/genkit/flows/**`, and `src/embedding/embedding.go`.

AI runs on **Firebase Genkit (Go)** with two providers in two separate Genkit instances:
- **Anthropic Claude** — the two generative flows (content-plan, post-assistant).
- **A local llama embedding server** — chunking, embedding, semantic retrieval (RAG).

Both flow families stream progress to the browser over SSE.

---

## 1. The two-runtime split

### Embedding instance — built once at boot, never rebuilt
In `server.New` (`src/server/server.go`): `embedding.WaitAndNewPlugin(ctx, EMBED_SERVER_URL,
...)` polls the embed server `/health` up to 12× @ 5s (~1 min) → `llama.Plugin`
(nil when `EMBED_SERVER_URL` empty). `genkit.Init(WithPlugins(llamaPlugin))` →
`embedding.RegisterFlows` defines the shared `ai.Embedder` and registers the `embedAsset` +
`processPDF` flows. The captured `embedder` is shared with the Anthropic runtime. The dev
Genkit UI sees only this instance.

### Anthropic flows runtime — hot-reloadable on key rotation
`genkitRuntime` (`src/server/genkit_runtime.go`) owns a *separate* `*genkit.Genkit` for the
Anthropic flows. The Anthropic plugin captures its API key at `genkit.Init` time, so a key
change requires re-init. `newGenkitRuntime` runs an initial `rebuild`, then:
```go
store.Subscribe(secrets.NameAnthropicAPIKey, func() { r.rebuild(ctx, store) })
```
`rebuild` (write-lock): read `anthropic_api_key`; `ErrNotFound` → **unavailable state**
(`contentPlanFn`/`postAssistantFn`/`g`/`plugin` = nil; boot allowed). Else build
`&anthropic.Anthropic{APIKey}`, `genkit.Init`, re-register both flows, atomically swap the
cached closures. In-flight requests keep running on the previous instance until they return.

### Why split
Quoting the code: *"Splitting the two instances keeps the Anthropic rebuild from disturbing
the embedder bound to the embedding instance."* An Anthropic key rotation (a frequent,
user-driven secrets-API event) must not tear down or re-poll the embed server.

### Rotation hook
`store.Set`/`Delete` call `notify(name)` synchronously → triggers `genkitRuntime.rebuild`.
So `PUT`/`DELETE /api/secrets/anthropic_api_key` rebuilds the flows with no restart.

---

## 2. Availability gating (503 with no key)

`IsAnthropicAvailable()` = `contentPlanFn != nil && postAssistantFn != nil`. Public
callbacks `GenerateDraft` / `RunPostAssistant` return `ErrAnthropicUnavailable`
(`"anthropic api key not configured"`) when their cached fn is nil. Handlers check **before**
opening the SSE stream:
- `CampaignsHandler.GenerateDraft` → `503 "content plan feature is not enabled"`.
- `PostsHandler.Assistant` → `503 "post assistant is not available"`.

`isContentPlanReady`/`isAssistantReady` are wired to `gkRuntime.IsAnthropicAvailable`.

---

## 3. Content-plan flow

Powers **`POST /api/campaigns/:id/generate-draft`**. Genkit flow `generateContentPlan`.
Package `src/genkit/flows/content_plan`.

### Inputs
- Request `ContentPlanRequest{CampaignID}`.
- `ContentPlanFlowConfig` from `config.Config`: `ModelID`, `MaxContextAssets`, `MaxContextChars`, `MaxOutputTokens`, `MaxPostsPerBatch`, `MaxParallelBatches`, `Embedder`, `Hub`, parsed `systemTmpl`/`userTmpl` from embedded `prompts/content_plan.tmpl`.
- `ContentPlanRepos`: `Campaigns`, `Assets`, `Chunks`, `Platforms`, `Posts`.

### Six steps (`runContentPlan`); each emits an SSE `step` event:
1. **`validateInput`** — loads campaign; requires `name`, `description`, `target_persona`, `key_messages`, `tone_guidelines`, `campaign_type_id`, `start_date`, `end_date`, ≥1 `target_platforms`, ≥1 phase; `start_date < end_date`, ≥24h range. Missing/bad → **`ValidationError`**. Captures `CreatedBy`.
2. **`resolveAssets`** — only when `UseAssets` (see §5.5).
3. **`resolvePlatforms`** — maps `target_platforms` to `resolvedPlatform`; per-entry `PostTypes` become `AllowedSlugs` (else all). Zero → `ValidationError`.
4. **`generatePosts`** — the substantive work (§3.3).
5. **`validateOutput`** / 6. **`persistDraftPosts`** — no-ops (validation/persistence are inline); still emit step events for client compat.

### Batching, generation, persistence
- **`planBatches`**: deterministically allocates *slots* across phases (remainder front-loaded), platforms within a phase, and contiguous `YYYY-MM-DD` date windows; chunks into batches of `MaxPostsPerBatch`. `nil` when `EstimatedPostCount ≤ 0` → single-shot fallback (model decides count).
- **`MaxPostsPerBatch`** (env, default 30): per-batch cap. **`MaxParallelBatches`** (env, default 5): concurrency guard against Anthropic rate limits.
- **Per-batch** (`generatePostsStreaming`): `genkit.GenerateStream` with model `"anthropic/"+ModelID`, system+user prompts, `MaxTokens` (`MaxOutputTokens`, 0→8192). A `jsonPostScanner` extracts complete JSON objects incrementally. Per `DraftPost`: `buildPostValidator` (platform id, allowed slug, publish date in `[start,end]`, phase id; fail → `warning` event + skip) → `persistOne` inserts as `models.Post` `status=draft` **immediately** (survives disconnects) → `post` event `{Post, Index, ID}` (`Index` = global slot index).
- **Stream-failure fallback:** re-issue a blocking `genkit.Generate`, strip fences, unmarshal, persist only not-yet-persisted positions (`persistedPositions`, first-write-wins). Both-fail → `AIError` (with survivors).
- **Parallel fan-out** (`runBatchesParallel`): semaphore (`maxParallel`); `onEvent` serialized behind a mutex. **Partial-success** aggregation: per-batch failure → warning; only **all batches failing** → `AIError`.

### Output `ContentPlanResponse{CampaignID, GeneratedAt, Posts []DraftPost, Warnings []string}`
`DraftPost`: `Title`, `Body` (≤500 runes), `ContentType`, `PlatformID`, `PublishDate`
(`YYYY-MM-DD`), `ToneNotes`, `PhaseID`, `AssetRefs []string`.

### SSE events (`SSEEventKind`)
`step` (`{Step, Status:"done"}`), `post` (`{Post, Index, ID}`), `warning`
(`{Message, Index?}`), `complete` (full `ContentPlanResponse`), `error` (`{Message, Code}`).
Handler maps: `ValidationError`→**400**, `AIError`→**502**, else **500** (conveyed as the
`error` event; HTTP is already 200).

### Hub finalization
`publishContentPlanFinalised` → topic `entity:campaign:<id>`, type
`content_plan_completed` (`{campaignId, postCount, warningCount}`) or `content_plan_failed`.

---

## 4. Post-assistant flow

Powers **`POST /api/posts/:id/assistant`**. Genkit flow `postAssistant`. Package
`src/genkit/flows/post_assistant`.

### Inputs
- Request `PostAssistantRequest{PostID, Instruction}` (empty instruction → `ValidationError`).
- `PostAssistantFlowConfig`: `ModelID`, `MaxOutputTokens`, `MaxTurns`, `Embedder`, `Hub`. Template `prompts/post_assistant.tmpl`.
- `PostAssistantRepos`: `Posts`, `Assets`, `Chunks`, `Campaigns`, `Versions`, `Messages`.

### Run sequence (`runPostAssistant`)
1. Load post (`ErrNoRows` → `ValidationError "post not found"`); capture `CreatedBy`.
2. Ensure initial version: if `CountByPostID==0` and content non-empty → `PostVersion{1, "Initial version", "user"}`.
3. Parallel: assemble context (`assembleContextCached`) + load history (`ListRecentByPostID(postID, 10)` → `ai` messages).
4. Inject per-request state (`postID`, `assetIDs: post.UsedAssetIDs`, repos, embedder).
5. Model call (streaming).
6. Assemble response from the scanner.
7. Persist conversation turn + versioning + post update.

### The model call
`genkit.Generate` with model `"anthropic/"+ModelID`, system = `SystemPrompt + ContextBlock`,
history messages, prompt = instruction, **tools** (below), `WithMaxTurns(MaxTurns||8)`,
streaming, `MaxTokens` (`MaxOutputTokens||64000`). Streaming is required (tool turns can
exceed the 10-min non-streaming timeout). It does **not** use `WithOutputType` — the flow
parses text itself with a tolerant scanner (§4.4) since Genkit's strict validator would
discard the whole response on any JSON blemish.

**Tools** (`defineTools`; per-request data via context):
- `listAssets` — attached assets (`id`, `name`, `type`, `chunkCount`).
- `getAssetChunks(AssetID, ChunkIDs?)` — chunks by id/all, packed under a 3000-token budget (`{Chunks, Truncated}`).
- `searchAssetChunks(AssetID, Query)` — **semantic RAG**: embed query, cosine-score chunks, keep `≥0.5`, sort desc, pack under budget. Errors if no embedder.
- `getCurrentContent` — latest post content as plain text.

**Streaming callback** fans out SSE events: text → `JSONStringScanner` watching
`"explanation"` / `"updatedContent"` → `explanation_delta` / `content_delta`
(`{Delta}`); completed tool requests → `tool_call` (`{Name, Input, Ref}`, deduped by Ref);
tool responses → `tool_result` (`{Name, Ref, OK:true}`, output bytes omitted).

### Tolerant parsing
`JSONStringScanner` (`scanner.go`) — hand-rolled incremental single-object JSON parser.
Streams decoded deltas of watched keys; after the stream, `Values()` tolerates trailing
commas, missing separators, preamble/postamble prose, literal newlines, truncation.
`PostAssistantResponse` reconstructed from `Values()`: `Explanation`, `UpdatedContent`,
`Action`, `SaveVersion`, `VersionNote`. Recovery: prose-only → salvage as `Explanation`,
`Action="declined"`; truncation (Anthropic `stop_reason=max_tokens`) → infer `"edited"` /
default explanation; genuinely empty → `AIError`.

### Context assembly + caching
`assembleContextCached` caches rendered system+context per post for `contextCacheTTL = 5m`,
keyed by `postFingerprint` (content + joined `UsedAssetIDs` + phase id). `assembleContext`
renders campaign fields, phase name/purpose, post content, and `buildAssetSummaries`
(per-asset title/type/chunk-count + ~200-char preview, combined cap `maxSummaryChars=4000`).

### Conversation history + versions
- User msg → `PostAssistantMessage{Role:"user", Content: instruction}`.
- Model msg → `{Role:"model", Content: JSON of {action, explanation, saveVersion, versionNote}}` — **excludes `updatedContent`** (avoids history bloat; UI restores badges on reload).
- Reload feeds back via `ListRecentByPostID(..., 10)`.
- **Versioning:** when `SaveVersion && Action=="edited"` → `PostVersion{latest+1, UpdatedContent, VersionNote, "assistant"}`.
- **Post update:** when `Action=="edited"` → `post.Content = UpdatedContent`; `Posts.Update`. Content is **Markdown** (frontend ↔ BlockNote JSON).

### Output + SSE events
`PostAssistantResponse{Explanation, UpdatedContent, Action ("edited"|"declined"),
SaveVersion, VersionNote}`. `SSEEventKind`: `explanation_delta`, `content_delta`,
`tool_call`, `tool_result`, `complete` (canonical response), `error`. Handler maps
`ValidationError`→400, `AIError`→502, else 500. `publishAssistantFinalised` → topic
`entity:post:<id>`, type `assistant_completed` / `assistant_failed`.

---

## 5. Embedding + RAG

### Model & store
- **Model:** `embeddinggemma-300m` via a llama-cpp-style embedserver at `EMBED_SERVER_URL`. `embedder.Name()` recorded on each chunk's `Model`.
- **Store:** SQLite (no external vector DB). Embeddings `[]float32` serialized little-endian (`encodeVector`/`DecodeVector`) into `assets_chunks.embedding`. Search = brute-force cosine in Go.

### Chunking (`chunker.go`)
`ChunkText`: ≤`MaxEmbedChars`(6000) → single chunk; else paragraph-aware split up to
`ChunkTarget`(5500) with `ChunkOverlap`(500)-char carry; oversized paragraphs split at word
boundaries. `EstimateTokens ≈ len/4`.

### Markdown embedding — the `onSave` callback (`embed_asset.go`)
`flows.Init` registers `embedAsset`. `NewAssetOnSaveCallback` → `OnMarkdownSave(assetID,
title, content)`, wired into `AssetsHandler` (fire-and-forget on save). `embedScheduler`
serializes embeds per asset and coalesces overlapping saves (latest pending wins). Per run:
status `processing` → (`ready`|`failed`), 10-min timeout. `embedAsset` prepends title,
chunks, skips word-less chunks (`hasWords`), embeds each chunk **individually** (single-doc
`/embed`; the batch endpoint fails with `n_tokens==0`), upserts via `Chunks.UpsertChunks`.

### PDF ingestion (`process_pdf.go`)
`flows.InitPDF` registers `processPDF`; `NewPDFProcessCallback` → `OnPDFProcess`
(fire-and-forget, 15-min timeout). See [`02-content-bank-assets.md §2`](./02-content-bank-assets.md) for the full step list.

### Asset status filtering + retrieval
- **Content-plan** (`assets.go`): `collectReadyCandidateIDs` excludes `failed`/`partial` assets. With an embedder, `rankAndPackChunks` embeds the campaign query (`Name + KeyMessages + Description`), fetches `Chunks.GetAllEmbedded`, keeps cosine `≥ minAssetSimilarity = 0.7`, sorts desc, greedily packs into `MaxContextChars`, grouped per asset in `ChunkIndex` order with page citations (`[p. N]`/`[pp. N–M]`). No embedder → creation-order fallback up to `MaxContextAssets`, ~800-char excerpts.
- **Post-assistant** is **tool-driven**: the model calls `searchAssetChunks` (threshold 0.5) / `getAssetChunks`, scoped to the post's `UsedAssetIDs`.

---

## 6. SSE streaming mechanics

Both handlers set `text/event-stream` + `Cache-Control: no-cache`, `Connection: keep-alive`,
`X-Accel-Buffering: no`, then `fasthttp.StreamWriter`. Each event: `event: <name>\ndata:
<json>\n\n`, flushed. `OnEventFunc` bridges flow events → SSE frames (mutex-serialized for
content-plan's parallel batches).

**`ssefix.go`** patches the openai-go SSE **decoder** on the **inbound** side (reading
Anthropic's stream). Anthropic sends comment-only keepalive pings (`: ping`); the default
decoder dispatches an event on every empty line → `json.Unmarshal` fails →
`"unexpected end of JSON input"` kills the stream. `skipEmptySSEDecoder` (registered in
`init()`) skips comment-only blocks (32 MiB scan buffer).

---

## 7. Config reference

See [README §3](./README.md#3-configuration--environment-variables) for the full table.
AI-relevant: `ANTHROPIC_API_KEY` (bootstrap; live key = secret `anthropic_api_key`),
`MODEL_ID` (`claude-sonnet-4-5-20250929`), `MAX_OUTPUT_TOKENS` (64000),
`MAX_ASSET_CONTEXT` (15), `MAX_CONTEXT_CHARS` (10000), `MAX_POSTS_PER_BATCH` (30),
`MAX_PARALLEL_BATCHES` (5), `EMBED_SERVER_URL` (empty disables embedding/RAG),
`GENKIT_ENV` (dev-mode toggle, only logged). Post-assistant `MaxTurns` defaults to 8 in
`runPostAssistant` (not env-driven).
