---
name: add-genkit-flow
description: Add a new Genkit-powered AI flow (Anthropic-backed) to the app — types, prompt template, run function, tools, context cache, HTTP handler, tests, and optional SSE streaming with per-field delta events. Use when the user asks to build an AI assistant, generator, classifier, or any other LLM-driven feature that needs prompt templates, structured output, tool use, or progress streaming to the UI.
tools: Read, Edit, Write, Glob, Grep, Bash
---

# Add Genkit Flow Skill

Scaffold a Genkit flow package with the canonical layering used by `post_assistant` and `content_plan`. Follow this exactly — the project already has two flows to diff against; deviating costs more than it saves. Canonical references:

- `src/genkit/flows/post_assistant/` — conversational, tools, SSE streaming with per-field deltas
- `src/genkit/flows/content_plan/` — multi-step pipeline, SSE streaming of typed events (step / post)

## Step 0: Clarify scope

Before writing any code, confirm with the user:

1. **Flow name** in camelCase (e.g. `postAssistant`, `generateContentPlan`). This becomes the Genkit flow name and the package name (snake_case: `post_assistant`).
2. **Structured output shape** — the fields the model must produce. Which are strings, which are enums, which are arrays?
3. **Streaming?** — does the client need live progress?
   - If yes: what granularity — **per-field text deltas** (like post_assistant) or **typed step/item events** (like content_plan)?
4. **Tools?** — what external data must the model fetch at its discretion (asset retrieval, search, etc.)? Tools are for model-driven lookups, not for mandatory context — put mandatory data in the prompt directly.
5. **Conversational?** — is there a message history per resource? If yes, where are messages persisted?
6. **HTTP route** — e.g. `POST /api/posts/:id/assistant`, `POST /api/campaigns/:id/generate-draft`.
7. **Error classes** — what counts as a 400 (bad input / not found) vs 502 (model failure) vs 500 (internal)?

Do not proceed until 1–4 and 6 are clear. Streaming granularity (3) drives half the code.

---

## Step 1: Package layout 

```
src/genkit/flows/<flow_name>/
├── types.go                    # request/response, SSE events, errors, repos bundle, flow config
├── flow.go                     # InitX, NewXCallback, emit helper
├── run.go                      # runX — the actual flow logic
├── context.go                  # prompt context assembly + fingerprint cache (if static context)
├── tools.go                    # ai.ToolRef bundle + implementations (if tools)
├── scanner.go                  # incremental JSON string-value scanner (if per-field streaming)
├── scanner_test.go             # scanner unit tests (pure Go, no Ginkgo)
└── prompts/
    └── <flow_name>.tmpl        # two {{define "system"}} + {{define "context"}} blocks
```

Server wiring lives in a thin file `src/server/<flow_name>.go` (mirrors `src/server/content_plan.go` and `src/server/post_assistant.go`). HTTP handler lives in `src/handlers/<resource>.go` — usually co-located with the parent resource (posts → posts.go, campaigns → campaigns.go).

---

## Step 2: Types (`types.go`)

Every flow needs these. Copy the template, then trim what you don't use.

```go
package <flow_name>

import (
    "github.com/firebase/genkit/go/ai"

    "github.com/content-control-center/app/src/repository"
)

// <FlowName>Request is the input to the flow.
type <FlowName>Request struct {
    // e.g. PostID string `json:"postId"`
    // e.g. Instruction string `json:"instruction"`
}

// <FlowName>Response is the structured output from the flow.
//
// Field order matters: Anthropic's structured-output decoding emits keys in
// the order declared by the jsonschema tags, which is the struct declaration
// order. Put user-visible streaming fields (explanation, content) first so
// they start arriving before metadata fields.
type <FlowName>Response struct {
    // e.g. Explanation    string `json:"explanation"    jsonschema:"description=..."`
    // e.g. UpdatedContent string `json:"updatedContent" jsonschema:"description=..."`
    // e.g. Action         string `json:"action"         jsonschema:"description=...,enum=edited,enum=declined"`
}

// <FlowName>Repos bundles all repository dependencies. Always bundle via a
// single struct — the flow takes ONE repos arg, not N.
type <FlowName>Repos struct {
    // e.g. Posts    repository.PostRepository
    // e.g. Assets   repository.AssetRepository
}

// <FlowName>FlowConfig holds static settings (model id, token caps, optional
// embedder). Not per-request state — that goes in context.WithValue.
type <FlowName>FlowConfig struct {
    ModelID         string
    MaxOutputTokens int64
    // MaxTurns caps tool-use round-trips. With tools, the model needs
    // N+1 turns to make N tool calls plus 1 final answer. 0 = sensible
    // default (8). Genkit's library-wide default is 5; we recommend 8 so
    // realistic asset-incorporation flows don't blow the cap.
    MaxTurns int
    Embedder ai.Embedder // nil = semantic search unavailable
}

// ── Errors (mapped to HTTP codes by the handler) ────────────────────────────

// ValidationError → HTTP 400. Use for bad input, missing records, permission.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// AIError → HTTP 502. Use only for model-call failures.
type AIError struct{ Msg string }

func (e *AIError) Error() string { return e.Msg }

// ── SSE event types (omit this block if the flow is not streaming) ──────────

type SSEEventKind string

const (
    // Pick the shape that fits the flow. Two common patterns:
    //   per-field-delta flow:   explanation_delta, content_delta, tool_call, tool_result
    //   multi-step flow:        step, post (or item)
    // Every streaming flow also emits: complete, error.
    SSEEvent<Kind>   SSEEventKind = "<kind>"
    SSEEventComplete SSEEventKind = "complete"
    SSEEventError    SSEEventKind = "error"
)

// DeltaEventPayload — used by per-field-delta flows.
type DeltaEventPayload struct {
    Delta string `json:"delta"`
}

// StepEventPayload — used by multi-step flows (one per named step).
type StepEventPayload struct {
    Step   string `json:"step"`
    Status string `json:"status"` // always "done" — start/fail states aren't useful
}

// ToolCallEventPayload / ToolResultEventPayload — used when the flow exposes
// model tool calls to the UI. Omit the tool's output bytes in the result
// event; the UI only needs a "done" signal to resolve the chip.
type ToolCallEventPayload struct {
    Name  string `json:"name"`
    Input any    `json:"input,omitempty"`
    Ref   string `json:"ref,omitempty"`
}

type ToolResultEventPayload struct {
    Name string `json:"name"`
    Ref  string `json:"ref,omitempty"`
    OK   bool   `json:"ok"`
}

type ErrorEventPayload struct {
    Message string `json:"message"`
    Code    int    `json:"code"` // HTTP semantic: 400, 502, 500
}

// OnEventFunc is an optional callback invoked as SSE events are produced.
// A nil OnEventFunc is valid — the flow runs silently (Genkit Dev UI path).
type OnEventFunc func(name SSEEventKind, data any)
```

**Rules:**
- `jsonschema:"description=..."` is mandatory on every response field — Anthropic uses these to constrain output.
- Keep `Embedder` even if unused today; adding it later is trivial and tools generally need it.
- Never make errors untyped — the handler dispatches on `*ValidationError` / `*AIError` via `errors.As`.
- Enum fields: `jsonschema:"...,enum=v1,enum=v2"` — comma-separated `enum=` tags.

---

## Step 3: Prompt template (`prompts/<flow_name>.tmpl`)

Two blocks: `system` (stable cached prefix — role, rules, schema) and `context` (variable per-request data). Anthropic's server-side prompt cache uses exact prefix matching, so the system block should be identical across calls for the same flow type.

```go-template
{{define "system"}}You are the <role>.

## Your role
- <bullet list of what it does>

## Response format
You MUST respond with a single JSON object matching this exact schema.
Field order matters — emit keys in exactly the sequence shown:
{
  "explanation": "...",
  "updatedContent": "..."
}

Respond ONLY with the JSON object. No markdown fences, no extra text.{{end}}

{{define "context"}}## <Variable section title>

**Field**: {{.FieldName}}
{{- if .OptionalField}}
**Optional**: {{.OptionalField}}
{{- end}}
{{end}}
```

**Rules:**
- Embed via `//go:embed prompts/<flow_name>.tmpl` in `flow.go`.
- The template data struct (`contextTemplateData`) lives in `context.go`.
- Always instruct the model to respond with raw JSON, no fences. The parser in `run.go` tolerates fences defensively, but the prompt should ask for clean output.
- Never reference fields the Go struct doesn't have — they'll silently disappear on unmarshal.

---

## Step 4: Context assembly (`context.go`) — if the flow has static prompt context

Use the fingerprint cache pattern. A plain string-equality check on one field is a bug waiting to happen (see the bug fixed in commit history where asset-list changes weren't invalidating the cache).

```go
package <flow_name>

import (
    "bytes"
    "context"
    "fmt"
    "strings"
    "sync"
    "text/template"
    "time"

    "github.com/content-control-center/app/src/models"
)

const contextCacheTTL = 5 * time.Minute

type assistantContext struct {
    SystemPrompt string // stable cached prefix
    ContextBlock string // variable per-request data
}

type contextCacheEntry struct {
    ctx         *assistantContext
    fingerprint string
    expiresAt   time.Time
}

var (
    contextCache   = map[string]*contextCacheEntry{}
    contextCacheMu sync.Mutex
)

// postFingerprint returns a stable key that changes whenever any
// prompt-affecting field of the resource changes. Add every field that
// feeds into the rendered context here — missing a field means stale
// prompts.
func <resource>Fingerprint(r *models.<Resource>) string {
    var b strings.Builder
    b.WriteString(r.Content)
    b.WriteByte('\x1f')
    b.WriteString(strings.Join(r.UsedAssetIDs, ","))
    b.WriteByte('\x1f')
    // ... more fields as needed
    return b.String()
}

func assembleContextCached(
    ctx context.Context,
    r *models.<Resource>,
    repos <FlowName>Repos,
    systemTmpl, contextTmpl *template.Template,
) (*assistantContext, error) {
    fp := <resource>Fingerprint(r)

    contextCacheMu.Lock()
    if entry, ok := contextCache[r.ID]; ok {
        if time.Now().Before(entry.expiresAt) && entry.fingerprint == fp {
            contextCacheMu.Unlock()
            return entry.ctx, nil
        }
        delete(contextCache, r.ID)
    }
    contextCacheMu.Unlock()

    actx, err := assembleContext(ctx, r, repos, systemTmpl, contextTmpl)
    if err != nil {
        return nil, err
    }

    contextCacheMu.Lock()
    contextCache[r.ID] = &contextCacheEntry{
        ctx:         actx,
        fingerprint: fp,
        expiresAt:   time.Now().Add(contextCacheTTL),
    }
    contextCacheMu.Unlock()

    return actx, nil
}
```

`assembleContext` renders the template from a `contextTemplateData` struct. Split independent I/O across goroutines with `sync.WaitGroup` when fetching multiple unrelated resources.

**Rules:**
- Unit-separator (`\x1f`) between fingerprint fields — no payload can smuggle itself into an adjacent slot.
- Every field read by the template MUST be in the fingerprint.
- TTL of 5 minutes aligns with Anthropic's server-side prompt cache TTL.
- Skip context.go entirely if the flow has no static prompt context (one-shot classifiers, for example).

---

## Step 5: Tools (`tools.go`) — if the model makes tool calls

Tools are model-driven lookups. Per-request state (IDs the model shouldn't have to guess) goes into `context.Context` via a package-private key; tools read it via `getRequestState`.

```go
package <flow_name>

import (
    "context"

    "github.com/firebase/genkit/go/ai"
    "github.com/firebase/genkit/go/genkit"
)

type ctxKey int

const requestStateKey ctxKey = iota

type requestState struct {
    // IDs the tool needs but shouldn't trust from the model
    postID   string
    assetIDs []string
    repos    <FlowName>Repos
    embedder ai.Embedder
}

func withRequestState(ctx context.Context, s *requestState) context.Context {
    return context.WithValue(ctx, requestStateKey, s)
}

func getRequestState(ctx context.Context) *requestState {
    return ctx.Value(requestStateKey).(*requestState)
}

type toolSet struct {
    listAssets        ai.ToolRef
    getAssetChunks    ai.ToolRef
    // ...
}

func defineTools(g *genkit.Genkit) *toolSet {
    list := genkit.DefineTool(g, "listAssets",
        "Returns the list of assets attached to the current resource.",
        func(ctx *ai.ToolContext, _ struct{}) ([]AssetInfo, error) {
            return toolListAssets(ctx)
        },
    )
    // ...
    return &toolSet{listAssets: list /* , ... */}
}
```

**Rules:**
- Tool names must match regex `^[a-zA-Z0-9_-]{1,64}$` (Anthropic constraint).
- Tool descriptions are read by the model — write them for an LLM audience, not a human reader.
- Tool inputs are JSON-schema'd via `jsonschema:"description=..."` tags on the input struct fields.
- Token-budget tool outputs aggressively (e.g., `chunkTokenBudget = 3000`). The model pays for every returned byte on every subsequent turn.
- Never put mandatory data in tools — if the model MUST see it, put it in the context block.

---

## Step 6: JSON parser (`scanner.go` + `scanner_test.go`) — use this for every flow

`JSONStringScanner` from `src/genkit/flows/post_assistant/scanner.go` is the canonical JSON parser for Anthropic structured output in this codebase. **Use it for every flow that consumes JSON from the model**, even non-streaming ones — it replaces `json.Unmarshal` and removes a whole class of production bugs.

Copy the file (`scanner.go` + `scanner_test.go`) verbatim into your flow package. The two `_test.go` groups travel with it: `TestScanner_*` covers streaming-delta behaviour and `TestValues_*` covers the drift patterns below.

### Why not `json.Unmarshal`?

Claude's JSON output drifts in production. We hit at least two distinct failures within hours of rolling out the obvious "ask for JSON, parse with `json.Unmarshal`" approach. The drift comes in many shapes; this table is the inventory we built up across post_assistant:

| Drift pattern | Example | `json.Unmarshal` | Scanner via `Values()` |
|---|---|---|---|
| Trailing comma | `{"a":"b",}` | ❌ hard fail | ✅ structural commas ignored at separator boundaries |
| Preamble / postamble prose | `Sure: {...}. Let me know!` | ❌ | ✅ `stPreamble` skips before `{`, `stDone` after `}` |
| Markdown code fence | ` ```json\n{...}\n``` ` | ❌ | ✅ same as above |
| Missing comma between string pairs | `{"a":"b" "c":"d"}` | ❌ | ✅ `stAfterValue` treats `"` as next-key start |
| Missing comma after bool/number | `{"saveVersion":true "note":"x"}` | ❌ | ✅ `stCollectLiteral` recovery |
| Literal newline inside string value | `{"x":"line1<LF>line2"}` (raw `\x0a`, not `\\n`) | ❌ | ✅ non-structural bytes are content |
| Unescaped tab / CR inside string | `"foo\x09bar"` | ❌ | ✅ same |
| Smart / curly quotes around strings | `{"a":"text"}` (`U+201C`/`U+201D`) | ❌ | partial — string treated as literal, raw fallback |
| Truncation mid-string | `{"a":"hel` (max_tokens hit) | ❌ | ✅ partial value returned |
| Truncation mid-literal | `{"saveVersion":tru` | ❌ | raw `"tru"` returned — caller treats as missing |
| Duplicate keys | `{"a":"first","a":"second"}` | last-wins | last-wins (same) |
| Nested objects | `{"meta":{...},"a":"b"}` | parsed | top-level only — nested values are absent from `Values()` |

The scanner's tolerance comes from walking the grammar character-by-character with explicit recovery states, not from regex post-processing. We tried the "tolerant pre-processor" approach (`stripTrailingCommas` + `extractJSONObject` + `insertMissingCommas` + `json.Unmarshal`) — it covered three patterns at the cost of fragile regex-on-JSON, and missed several others. The scanner is one piece of code that handles every row above; adding the next pattern (when one inevitably appears) is a state-machine tweak, not another regex.

### Why not `ai.WithOutputType(<Response>{})`?

Genkit's `WithOutputType` enables Anthropic's native structured output **and** a post-generation strict schema validator on the genkit side. The validator returns `(nil, err)` and discards the entire response on any blemish — meaning a single trailing comma takes down the whole turn, and we can't recover the raw text to repair it. The structured-output _hint_ (the schema attached to the request) does nudge the model, but in practice Claude 4.x with a clear "respond ONLY with the JSON object" prompt + scanner-based extraction is sufficient and dramatically more resilient.

If you observe systematic prose-drift in the wild (the model returning narrative instead of JSON for a specific flow), revisit the trade-off — but reach for prompt strengthening before reintroducing `WithOutputType`.

### Scanner API

```go
// watched determines which keys fire onDelta during streaming. Pass nil
// for both args if the flow doesn't stream — Values() still works.
scanner := NewJSONStringScanner(
    []string{"explanation", "updatedContent"},
    func(key, delta string) {
        switch key {
        case "explanation":
            emit(onEvent, SSEEventExplanationDelta, DeltaEventPayload{Delta: delta})
        case "updatedContent":
            emit(onEvent, SSEEventContentDelta, DeltaEventPayload{Delta: delta})
        }
    },
)

// Feed every text chunk inside the WithStreaming callback.
scanner.Push(part.Text)

// After genkit.Generate returns, snapshot the parsed values.
vals := scanner.Values()

// Optional: the full raw text, for debugging.
raw := scanner.FullText()
```

`Values()` returns `map[string]any` with these typed values:

| Stored kind | Returned Go type | Notes |
|---|---|---|
| string literal | `string` | JSON escapes resolved (`\n`, `\uXXXX`, surrogate pairs) |
| `true` / `false` | `bool` | |
| `null` | `nil` | Map key still present |
| numeric literal | `float64` | `strconv.ParseFloat` — covers ints and floats |
| unparseable literal | `string` (raw trimmed) | E.g. truncated `"tru"` — caller decides if missing |
| nested object / array | absent from map | We don't extract these — keys are silently skipped |
| key never seen | absent from map | E.g. when the model omits an optional field |

### Sanity-check the extraction

The scanner returns whatever it saw. If the model produced something so broken that no top-level keys were observed, `Values()` returns an empty map and you'd silently persist garbage. After populating your response struct, **always verify your required fields are non-empty** and bail with a logged `AIError` otherwise:

```go
if result.RequiredField1 == "" || result.RequiredField2 == "" {
    log.Printf("<flow>[%s]: scanner failed to extract required fields (len=%d): %.500s",
        req.<ID>, len(scanner.FullText()), scanner.FullText())
    return nil, &AIError{Msg: "model response did not contain the expected fields"}
}
```

This is the only way the user sees an `AIError` from the parse path under the scanner approach — and it means the model's output didn't even resemble the expected JSON. Log captures the raw text so you can diagnose new drift shapes from production.

### What still needs your attention per flow

The scanner is verbatim-portable, but two things are flow-specific:

1. **`watched` list** — which keys do you want streaming-delta callbacks for. Typically the user-visible long-text fields (an `explanation`, a `body`, a `commentary`).
2. **The unpacking from `Values()` into your `<FlowName>Response`** — type-asserted per field, with a sanity check on required fields.

Everything else (state machine, escape decoding, UTF-8 carry, recovery states) is the same across every flow and shouldn't need touching.

---

## Step 7: Run function (`run.go`)

The heart of the flow. This template is the per-field-streaming-with-tools case — trim for simpler flows.

```go
package <flow_name>

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "strings"
    "sync"
    "text/template"
    "time"

    "github.com/anthropics/anthropic-sdk-go"
    "github.com/firebase/genkit/go/ai"
    "github.com/firebase/genkit/go/genkit"
)

func run<FlowName>(
    ctx context.Context,
    g *genkit.Genkit,
    req <FlowName>Request,
    cfg <FlowName>FlowConfig,
    repos <FlowName>Repos,
    systemTmpl, contextTmpl *template.Template,
    tools *toolSet, // omit if no tools
    onEvent OnEventFunc,
) (*<FlowName>Response, error) {
    start := time.Now()

    // ── Validate input + load resource ──────────────────────────────────────
    if req.<Field> == "" {
        return nil, &ValidationError{Msg: "<field> is required"}
    }
    // repos.X.GetByID(ctx, id) — wrap sql.ErrNoRows as ValidationError

    // ── Assemble context + independent I/O in parallel ──────────────────────
    var actx *assistantContext
    var ctxErr error
    var history []*ai.Message
    var histErr error

    var wg sync.WaitGroup
    wg.Add(2)
    go func() {
        defer wg.Done()
        actx, ctxErr = assembleContextCached(ctx, resource, repos, systemTmpl, contextTmpl)
    }()
    go func() {
        defer wg.Done()
        // load history if conversational; otherwise delete this goroutine
    }()
    wg.Wait()

    if ctxErr != nil {
        return nil, fmt.Errorf("assemble context: %w", ctxErr)
    }
    if histErr != nil {
        return nil, fmt.Errorf("load history: %w", histErr)
    }

    // ── Inject per-request state for tools ──────────────────────────────────
    ctx = withRequestState(ctx, &requestState{ /* ... */ })

    // ── Call model ──────────────────────────────────────────────────────────
    maxTokens := cfg.MaxOutputTokens
    if maxTokens == 0 { maxTokens = 8192 }
    maxTurns := cfg.MaxTurns
    if maxTurns == 0 { maxTurns = 8 } // see Step 2 FlowConfig comment
    modelName := "anthropic/" + cfg.ModelID
    systemBlock := actx.SystemPrompt + "\n\n" + actx.ContextBlock

    // Scanner + tool dedup state live outside the callback so they survive
    // across tool-use round-trips within the same Generate call.
    scanner := NewJSONStringScanner(
        []string{"explanation", "updatedContent"},
        func(key, delta string) {
            switch key {
            case "explanation":
                emit(onEvent, SSEEventExplanationDelta, DeltaEventPayload{Delta: delta})
            case "updatedContent":
                emit(onEvent, SSEEventContentDelta, DeltaEventPayload{Delta: delta})
            }
        },
    )
    emittedToolCalls := map[string]bool{}
    emittedToolResults := map[string]bool{}

    streamCb := func(_ context.Context, chunk *ai.ModelResponseChunk) error {
        if chunk == nil || chunk.Aggregated {
            return nil
        }
        for _, part := range chunk.Content {
            switch {
            case part.IsText():
                scanner.Push(part.Text)
            case part.IsToolRequest():
                tr := part.ToolRequest
                if tr == nil || tr.Partial || emittedToolCalls[tr.Ref] {
                    continue
                }
                emittedToolCalls[tr.Ref] = true
                emit(onEvent, SSEEventToolCall, ToolCallEventPayload{
                    Name: tr.Name, Input: tr.Input, Ref: tr.Ref,
                })
            case part.IsToolResponse():
                tr := part.ToolResponse
                if tr == nil || emittedToolResults[tr.Ref] {
                    continue
                }
                emittedToolResults[tr.Ref] = true
                emit(onEvent, SSEEventToolResult, ToolResultEventPayload{
                    Name: tr.Name, Ref: tr.Ref, OK: true,
                })
            }
        }
        return nil
    }

    resp, err := genkit.Generate(ctx, g,
        ai.WithModelName(modelName),
        ai.WithSystem(systemBlock),
        ai.WithMessages(history...),              // omit if not conversational
        ai.WithPrompt(req.<Prompt>),
        ai.WithTools(tools.listAssets /* , ... */), // omit if no tools
        ai.WithMaxTurns(maxTurns),                // see Step 2 FlowConfig + gotcha #12
        ai.WithStreaming(streamCb),
        ai.WithConfig(anthropic.MessageNewParams{
            MaxTokens: maxTokens,
        }),
        // NB: do NOT add ai.WithOutputType — see Step 6 "Why not ai.WithOutputType"
    )
    if err != nil {
        return nil, &AIError{Msg: fmt.Sprintf("model call failed: %v", err)}
    }

    // ── Assemble response from scanner ──────────────────────────────────────
    // The scanner has been processing every chunk in the streaming callback
    // above. Values() returns the parsed top-level fields without going
    // through encoding/json — see Step 6 for the drift patterns this fixes.
    vals := scanner.Values()
    result := <FlowName>Response{}
    if s, ok := vals["explanation"].(string); ok    { result.Explanation = s }
    if s, ok := vals["updatedContent"].(string); ok { result.UpdatedContent = s }
    if s, ok := vals["action"].(string); ok         { result.Action = s }
    if b, ok := vals["saveVersion"].(bool); ok      { result.SaveVersion = b }
    if s, ok := vals["versionNote"].(string); ok    { result.VersionNote = s }

    // Required-field sanity check. If the scanner saw nothing recognisable,
    // we'd otherwise silently persist a zero-valued struct.
    if result.Explanation == "" || result.Action == "" {
        raw := scanner.FullText()
        log.Printf("<flow>[%s]: scanner failed to extract required fields (len=%d): %.500s",
            req.<ID>, len(raw), raw)
        return nil, &AIError{Msg: "model response did not contain the expected fields"}
    }

    // ── Apply domain conversions + persist ──────────────────────────────────
    // e.g. Markdown → BlockNote JSON, create version snapshot, update resource
    // Persist both the user turn and the model turn if conversational.

    log.Printf("<flow_name>[%s]: done in %s", req.<ID>, time.Since(start).Round(time.Millisecond))

    // Canonical final event. Deltas before this were preview-only.
    emit(onEvent, SSEEventComplete, &result)

    return &result, nil
}
```

**Rules:**
- `genkit.Generate` + `WithStreaming` callback, NOT `genkit.GenerateStream`. The former preserves tool round-tripping naturally; the latter is just a wrapper that yields chunks via an iterator (less control).
- **No `ai.WithOutputType`.** It enables a strict post-generation validator that hard-fails on common Claude drift (trailing comma, etc.) and discards the response. See Step 6 for the rationale and the comprehensive drift table.
- **Use `scanner.Values()` instead of `json.Unmarshal`.** Same reason — full character-level tolerance, partial response support on truncation, and one less moving part. Sanity-check required fields after extraction.
- Skip chunks where `Aggregated == true` — those are recap frames that would double-feed the scanner.
- Dedupe tool events by `Ref` with a local map. Emit tool_call only when `!Partial` (complete request).
- Log the full prompt once per call for debugging: `log.Printf("<flow>: system prompt:\n%s", systemPrompt)`. Do NOT log the response — it may contain user data. Exception: log raw text (truncated to ~500 chars) when the required-field sanity check trips, so new drift shapes are diagnosable.
- Wrap `sql.ErrNoRows` as `ValidationError{Msg: "<resource> not found"}`. Wrap model errors as `AIError`. Never return raw `errors.New` from the top level — the handler won't know the HTTP code.

---

## Step 8: Flow file (`flow.go`)

```go
package <flow_name>

import (
    "context"
    "embed"
    "fmt"
    "text/template"

    "github.com/firebase/genkit/go/core"
    "github.com/firebase/genkit/go/genkit"
)

//go:embed prompts/<flow_name>.tmpl
var promptFS embed.FS

var <FlowName>Flow *core.Flow[<FlowName>Request, *<FlowName>Response, struct{}]

// runner bypasses the Genkit flow wrapper so we can thread OnEventFunc
// through for SSE streaming. The wrapped Flow is for the Dev UI only.
var <flowName>Runner func(ctx context.Context, req <FlowName>Request, onEvent OnEventFunc) (*<FlowName>Response, error)

func Init<FlowName>(g *genkit.Genkit, cfg <FlowName>FlowConfig, repos <FlowName>Repos) error {
    raw, err := promptFS.ReadFile("prompts/<flow_name>.tmpl")
    if err != nil {
        return fmt.Errorf("load <flow_name>.tmpl: %w", err)
    }
    tmpl, err := template.New("<flow_name>").Parse(string(raw))
    if err != nil {
        return fmt.Errorf("parse <flow_name>.tmpl: %w", err)
    }
    systemTmpl := tmpl.Lookup("system")
    contextTmpl := tmpl.Lookup("context")
    if systemTmpl == nil || contextTmpl == nil {
        return fmt.Errorf("<flow_name>.tmpl must define both system and context blocks")
    }

    tools := defineTools(g) // omit if no tools

    <FlowName>Flow = genkit.DefineFlow(g, "<flowName>",
        func(ctx context.Context, req <FlowName>Request) (*<FlowName>Response, error) {
            return run<FlowName>(ctx, g, req, cfg, repos, systemTmpl, contextTmpl, tools, nil)
        },
    )

    <flowName>Runner = func(ctx context.Context, req <FlowName>Request, onEvent OnEventFunc) (*<FlowName>Response, error) {
        return run<FlowName>(ctx, g, req, cfg, repos, systemTmpl, contextTmpl, tools, onEvent)
    }

    return nil
}

// New<FlowName>Callback returns the callback the HTTP handler will invoke.
// Pass a non-nil OnEventFunc for SSE streaming, nil for a silent call.
func New<FlowName>Callback() func(ctx context.Context, req <FlowName>Request, onEvent OnEventFunc) (*<FlowName>Response, error) {
    return func(ctx context.Context, req <FlowName>Request, onEvent OnEventFunc) (*<FlowName>Response, error) {
        return <flowName>Runner(ctx, req, onEvent)
    }
}

// emit is a no-op when onEvent is nil. Keeps run.go free of nil checks.
func emit(onEvent OnEventFunc, name SSEEventKind, data any) {
    if onEvent != nil {
        onEvent(name, data)
    }
}
```

**Rules:**
- Two registration paths: `DefineFlow` (shows up in Genkit Dev UI for exploration, runs silently) and the direct runner closure (streams events).
- Package init via `InitX(g, cfg, repos) error` — called once from server.go after `genkit.Init`.
- Never make the callback receiver a struct — a closure returned from `NewXCallback()` is simpler and harder to misuse.

---

## Step 9: Server wiring (`src/server/<flow_name>.go` + edits to `src/server/server.go`)

Small adapter file to keep server.go clean:

```go
// src/server/<flow_name>.go
package server

import (
    "context"
    "fmt"

    "github.com/firebase/genkit/go/ai"
    "github.com/firebase/genkit/go/genkit"

    "github.com/content-control-center/app/src/config"
    "github.com/content-control-center/app/src/genkit/flows/<flow_name>"
)

func init<FlowName>(
    g *genkit.Genkit,
    cfg *config.Config,
    embedder ai.Embedder,
    repos <flow_name>.<FlowName>Repos,
) (func(ctx context.Context, req <flow_name>.<FlowName>Request, onEvent <flow_name>.OnEventFunc) (*<flow_name>.<FlowName>Response, error), error) {
    flowCfg := <flow_name>.<FlowName>FlowConfig{
        ModelID:         cfg.ModelID,
        MaxOutputTokens: cfg.MaxOutputTokens,
        Embedder:        embedder,
    }
    if err := <flow_name>.Init<FlowName>(g, flowCfg, repos); err != nil {
        return nil, fmt.Errorf("init <flow_name> flow: %w", err)
    }
    return <flow_name>.New<FlowName>Callback(), nil
}
```

In `src/server/server.go`, add the callback var declaration near the other callbacks and register only if the Anthropic key is configured:

```go
var <flowName>Callback func(context.Context, <flow_name>.<FlowName>Request, <flow_name>.OnEventFunc) (*<flow_name>.<FlowName>Response, error)

// ... later, inside the Anthropic-key block:
<flowName>Repos := <flow_name>.<FlowName>Repos{ /* ... */ }
<flowName>Callback, err = init<FlowName>(g, cfg, embedder, <flowName>Repos)
if err != nil {
    return nil, err
}

// ... at handler registration:
handlers.New<Resource>Handler(/* existing args */, <flowName>Callback).Register(app)
```

**Rules:**
- Gate registration behind `cfg.AnthropicAPIKey != ""`. Leaves the callback nil → handler emits 503.
- Use ONE `genkit.Init(ctx, genkit.WithPlugins(plugins...))` for all plugins — see `CON-42: Consolidate Genkit into single instance` in git log for why.
- Use the native `plugins/anthropic` import path, NOT `plugins/compat_oai/anthropic`. The compat plugin rejects `anthropic.MessageNewParams` as an unknown config type.

---

## Step 10: HTTP handler (`src/handlers/<resource>.go`)

Two shapes: JSON (non-streaming) or SSE (streaming). Use SSE if the flow emits events.

### SSE handler template

```go
// imports you'll need:
//   "bufio"
//   "context"
//   "encoding/json"
//   "errors"
//   "fmt"
//   "github.com/valyala/fasthttp"
//   "github.com/content-control-center/app/src/genkit/flows/<flow_name>"

// <Verb> godoc
// @Summary      <Resource> <verb> (SSE)
// @Description  Streams progress via Server-Sent Events.
// @Description  Events: <list of event names> carry <shape>.
// @Description  "complete" carries the final <FlowName>Response.
// @Description  "error" carries {"message":"...","code":<http_code>}.
// @Tags         <resource>
// @Accept       json
// @Produce      text/event-stream
// @Security     CookieAuth
// @Param        id    path      string           true  "<Resource> Sqid"
// @Param        body  body      <verbRequest>    true  "Payload"
// @Success      200  "SSE stream"
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      503  {object}  map[string]string
// @Router       /api/<resource>/{id}/<verb> [post]
func (h *<Resource>Handler) <Verb>(c *fiber.Ctx) error {
    if h.<flowName> == nil {
        return fiber.NewError(fiber.StatusServiceUnavailable, "<flow> is not available")
    }
    var req <verb>Request
    if err := c.BodyParser(&req); err != nil {
        return fiber.NewError(fiber.StatusBadRequest, err.Error())
    }
    if err := validate.Struct(&req); err != nil {
        return fiber.NewError(fiber.StatusBadRequest, validationError(err).Error())
    }

    c.Set("Content-Type", "text/event-stream")
    c.Set("Cache-Control", "no-cache")
    c.Set("Connection", "keep-alive")
    c.Set("X-Accel-Buffering", "no")

    id := c.Params("id")
    payload := req // capture before the stream writer starts
    runner := h.<flowName>

    c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
        writeEvent := func(event string, data any) {
            b, _ := json.Marshal(data)
            fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
            _ = w.Flush()
        }

        onEvent := <flow_name>.OnEventFunc(func(name <flow_name>.SSEEventKind, data any) {
            writeEvent(string(name), data)
        })

        // NB: use context.Background() here — the fiber ctx is cancelled
        // the moment the handler returns, which happens before the stream
        // writer runs.
        _, err := runner(context.Background(), <flow_name>.<FlowName>Request{
            <ID>: id,
            <Field>: payload.<Field>,
        }, onEvent)
        if err != nil {
            code := fiber.StatusInternalServerError
            msg := err.Error()
            var ve *<flow_name>.ValidationError
            var ae *<flow_name>.AIError
            switch {
            case errors.As(err, &ve):
                code = fiber.StatusBadRequest
                msg = ve.Msg
            case errors.As(err, &ae):
                code = fiber.StatusBadGateway
                msg = ae.Msg
            }
            writeEvent(string(<flow_name>.SSEEventError), <flow_name>.ErrorEventPayload{Message: msg, Code: code})
            return
        }
        // "complete" was emitted by the runner itself; nothing more to write.
    }))

    return nil
}
```

**Rules:**
- `context.Background()` inside the stream writer — the fiber request ctx is cancelled the moment the handler returns `nil`, which is BEFORE the stream writer runs.
- Capture `id` / `req` into local variables before starting the stream writer (fiber may reuse its buffers after return).
- Error mapping is `errors.As` against the typed errors from `types.go`. Add new error types there if you need a new HTTP code.
- `X-Accel-Buffering: no` tells nginx/upstream proxies to flush immediately.
- Thread the callback through the handler struct constructor (see content_plan + post_assistant for the pattern).

### JSON handler shape (non-streaming flow)

If the flow doesn't stream, the handler is trivial:

```go
resp, err := h.<flowName>(c.Context(), <flow_name>.<FlowName>Request{ /* ... */ }, nil /* onEvent */)
if err != nil {
    var ve *<flow_name>.ValidationError
    var ae *<flow_name>.AIError
    switch {
    case errors.As(err, &ve):
        return fiber.NewError(fiber.StatusBadRequest, ve.Msg)
    case errors.As(err, &ae):
        return fiber.NewError(fiber.StatusBadGateway, ae.Msg)
    }
    return err
}
return c.JSON(resp)
```

---

## Step 11: Tests (`src/handlers/<resource>_test.go`)

Handler-layer only. Real in-memory SQLite via `mustOpenTestDBWithMigrations()`. Stub the flow callback with a function that emits synthetic events.

```go
// inside the existing Describe block:
Context("with a stub <flowName> callback", func() {
    buildStubApp := func(
        stub func(context.Context, <flow_name>.<FlowName>Request, <flow_name>.OnEventFunc) (*<flow_name>.<FlowName>Response, error),
    ) (*fiber.App, *http.Cookie, string) {
        stubApp := fiber.New(fiber.Config{ /* usual error handler */ })
        // ... repos + auth + handlers
        handlers.New<Resource>Handler(/* ... */, stub).Register(stubApp)
        // ... seed user + login + create parent resource; return (app, authCookie, resourceID)
    }

    type sseEvent struct{ event, data string }
    parseSSE := func(body io.Reader) []sseEvent {
        var events []sseEvent
        scanner := bufio.NewScanner(body)
        var curEvent, curData string
        for scanner.Scan() {
            line := scanner.Text()
            switch {
            case strings.HasPrefix(line, "event: "):
                curEvent = strings.TrimPrefix(line, "event: ")
            case strings.HasPrefix(line, "data: "):
                curData = strings.TrimPrefix(line, "data: ")
            case line == "":
                if curEvent != "" { events = append(events, sseEvent{curEvent, curData}) }
                curEvent, curData = "", ""
            }
        }
        return events
    }

    It("streams the expected event sequence", func() {
        final := &<flow_name>.<FlowName>Response{ /* ... */ }
        stub := func(_ context.Context, _ <flow_name>.<FlowName>Request, onEvent <flow_name>.OnEventFunc) (*<flow_name>.<FlowName>Response, error) {
            onEvent(<flow_name>.SSEEvent<X>, <flow_name>.DeltaEventPayload{Delta: "..."})
            onEvent(<flow_name>.SSEEventComplete, final)
            return final, nil
        }
        stubApp, cookie, id := buildStubApp(stub)

        req := /* POST to /api/<resource>/<id>/<verb> */
        resp, _ := stubApp.Test(req, 5000)
        Expect(resp.StatusCode).To(Equal(200))
        Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("text/event-stream"))

        events := parseSSE(resp.Body)
        names := make([]string, len(events))
        for i, e := range events { names[i] = e.event }
        Expect(names).To(Equal([]string{"<x>", "complete"}))
    })

    It("emits an error event on ValidationError", func() {
        stub := func(_ context.Context, _ <flow_name>.<FlowName>Request, _ <flow_name>.OnEventFunc) (*<flow_name>.<FlowName>Response, error) {
            return nil, &<flow_name>.ValidationError{Msg: "bad input"}
        }
        // ... POST, parse SSE, expect 1 event with event=="error" and data containing `"code":400`
    })
})
```

**Rules:**
- Never mock the DB. Use `mustOpenTestDBWithMigrations()` per the project's testing conventions.
- Tests at the handler layer only — don't test the flow's internal functions directly (the scanner is the one exception; unit tests for pure logic are fine).
- Integration tests for the real model live under `src/integration/` with the `//go:build integration` tag — they need `ANTHROPIC_API_KEY` and the native `plugins/anthropic` plugin.

---

## Step 12 (optional): Frontend wiring

If there's a React prototype, use a `ReadableStream` reader — `EventSource` doesn't support POST bodies.

```jsx
const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
    credentials: "include",
    body: JSON.stringify(payload),
});
if (!res.ok || !res.body) throw new Error(`Request failed (${res.status})`);

const reader = res.body.getReader();
const decoder = new TextDecoder();
let buf = "";

const flushFrames = () => {
    let idx;
    while ((idx = buf.indexOf("\n\n")) >= 0) {
        const frame = buf.slice(0, idx);
        buf = buf.slice(idx + 2);
        let event = "", dataStr = "";
        for (const line of frame.split("\n")) {
            if (line.startsWith("event: ")) event = line.slice(7);
            else if (line.startsWith("data: ")) dataStr = line.slice(6);
        }
        if (event) handleEvent(event, dataStr);
    }
};

while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    flushFrames();
}
buf += decoder.decode();
flushFrames();
```

**Patterns for per-field-delta streaming UIs:**

- Store deltas as an array of chunks, not a concatenated string. React's index keys preserve already-mounted spans so only newly-appended spans re-run a fade-in animation (existing chunks stay put). See `prototyping/CON-42-post-assistant/src/components/PostContent.jsx::StreamingPreview`.
- Render a spinner next to the assistant bubble driven by a `streaming: true` flag on the message, flipped to false on `complete` / `error` / fetch catch.
- Expose tool calls as chips with a `running` → `done` state machine keyed on tool `Ref`. See `src/components/ChatPanel.jsx` for the exact render.

---

## Gotchas (read this before you start)

1. **Native Anthropic plugin, not compat.** `import "github.com/firebase/genkit/go/plugins/anthropic"`. The `plugins/compat_oai/anthropic` shim rejects `anthropic.MessageNewParams` as an unknown config type — the error is `unexpected config type: anthropic.MessageNewParams` which is misleading because the type name in the error matches the supported type (different package with the same short name).

2. **Prompt example ordering controls streaming order.** Without `WithOutputType` (see #11) the model isn't constrained to a server-side schema — it tracks the JSON object example you put in the prompt. Put user-visible fields (explanation, content) before metadata (flags, enums) in the example so they start streaming first. Struct field order in `<FlowName>Response` only affects how Go marshals the response when persisting to history — it does NOT affect the model's emission order anymore.

3. **Fingerprint every prompt field.** The context cache keyed only on one field (e.g. `post.Content`) silently returns stale data when any other prompt-affecting field changes. Use the unit-separator fingerprint pattern. When adding a new prompt field, update the fingerprint helper in the same commit.

4. **Dedupe tool events by Ref.** Anthropic streams partial tool requests as the input JSON is built. Emit `tool_call` only when `Partial == false`. Use a local map to dedupe by `Ref` in case the same complete tool use arrives in a later chunk.

5. **`context.Background()` in SSE handlers.** The fiber request ctx is cancelled the moment the handler returns `nil`, which happens BEFORE `SetBodyStreamWriter` runs its callback. Passing `c.Context()` to the runner will make every streaming call fail with context-cancelled after ~0ms.

6. **`Aggregated` chunks double-feed.** Check `chunk.Aggregated == false` before feeding text into the scanner. Aggregated chunks are recap frames that would append the full response text a second time and corrupt scanner state.

7. **Skip `partial` tool requests for UI.** `part.ToolRequest.Partial == true` means the model is still building the tool's input JSON. If you emit these, the UI sees a flicker of half-built inputs.

8. **Single `genkit.Init` for all plugins.** Flow discovery and the Dev UI break if plugins are split across Init calls. See commit `993fb8b CON-42: Fix flow discovery by passing all plugins to single genkit.Init`.

9. **Never log the model response.** It may contain user data. Log tokens / finish reason / duration, not text.

10. **Real DB in tests.** No mocks. `mustOpenTestDBWithMigrations()`. Tests at the handler layer, stubbing only the flow callback.

11. **`ai.WithOutputType` and `json.Unmarshal` are both traps.** They look like the obvious tools and they hard-fail on common Claude JSON drift. Use the `JSONStringScanner` for parsing — every flow, streaming or not. See Step 6 for the full table of drift patterns the scanner handles vs. what `json.Unmarshal` chokes on.

12. **Size `MaxTurns` for the flow's tool depth.** With tools enabled, `MaxTurns=N` allows up to `N-1` tool calls plus one final answer — the model needs an extra turn after its last tool call to produce the response. Genkit's library default is 5; we found 3 catastrophically low (asset-incorporation flows blow it on the second tool call) and recommend **8** as the FlowConfig default. Symptoms of too low: `model call failed: exceeded maximum tool call iterations (N)`. Symptoms of too high: a buggy prompt with tools can loop indefinitely (rare in practice — Claude is good at terminating). Tune via `<FlowName>FlowConfig.MaxTurns`; leaving it at 0 picks the default in `run.go`.

13. **Authorization before lookup.** If the flow operates on a user-owned resource, check ownership BEFORE returning 404 — never reveal whether an id exists to an unauthorized caller. The handler should 403 for "exists but not yours" and 404 for "doesn't exist or not yours" (whichever the project convention dictates — check `CLAUDE.md` / memory).

---

## Step 13: Verify

```bash
/usr/local/go/bin/go build ./src/...
/usr/local/go/bin/go vet ./src/...
/usr/local/go/bin/go test -count=1 ./src/genkit/flows/<flow_name>/... ./src/handlers/...
/usr/local/go/bin/go build -tags integration ./src/integration/...   # if you added an integration test
```

Fix any compile or test errors before reporting success. Do not claim success without a green run.

---

## Checklist (complete in order)

- Prompt template created (`src/genkit/flows/<flow_name>/prompts/<flow_name>.tmpl`) with `system` + `context` blocks; user-visible fields appear first in the example JSON
- `types.go` created — request, response, config, repos, errors, SSE events (if streaming)
- **`scanner.go` + `scanner_test.go` copied verbatim** from `src/genkit/flows/post_assistant/` — every flow uses the scanner, even non-streaming ones
- `tools.go` created (if tools needed) — tool names match regex, descriptions written for LLM audience
- `context.go` created with `fingerprint` cache (if static context) — every prompt field in the fingerprint
- `run.go` created — streaming callback feeds the scanner, **no `ai.WithOutputType`**, response struct populated from `scanner.Values()` with required-field sanity check, tool dedup, error mapping
- `flow.go` created — `InitX`, `NewXCallback`, `emit` helper
- `src/server/<flow_name>.go` adapter created
- `src/server/server.go` edited — callback var + `initX` call + handler registration gated on API key
- HTTP handler added to `src/handlers/<resource>.go` — SSE or JSON shape, swagger comments, service-unavailable branch
- Callback threaded through `New<Resource>Handler` constructor signature
- Handler test added (`src/handlers/<resource>_test.go`) — stub callback, SSE frame parser, happy path + error path
- (Optional) Integration test added under `src/integration/` with `//go:build integration` tag
- (Optional) React prototype wiring — SSE reader, per-chunk render, tool chips, spinner
- `go build ./src/...` clean
- `go test -count=1 ./src/...` green — including all `TestScanner_*`, `TestValues_*`, and `TestTrimIncompleteUTF8`
