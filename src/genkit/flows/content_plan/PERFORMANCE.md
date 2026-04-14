# Content plan generation — performance notes

Current observed latency: **~3 minutes** end-to-end (Haiku 4.5, ~30 posts).

Root cause: a single synchronous model call generates ~5–6k output tokens sequentially.
Input token volume (prompt, pieces) is a secondary factor.

---

## Option 1 — Parallel per-platform calls ★ highest ROI

Split the single `generatePosts` call into N concurrent calls, one per platform.
Each call generates only the posts for that platform (~1/N output tokens); results are merged.

```
platforms: [linkedin, instagram, facebook]
  → goroutine 1: generate linkedin posts   ┐
  → goroutine 2: generate instagram posts  ├─ concurrent
  → goroutine 3: generate facebook posts   ┘
  → merge & return
```

Implementation: fan out over `platforms` in `generatePosts` using `golang.org/x/sync/errgroup`.
Files touched: `generate.go` only.
Expected speedup: ~Nx where N = number of target platforms (~3× for 3 platforms).

---

## Option 2 — Reduce excerpt length

Current: `maxChars = 800` (~200 tokens) per piece.
Proposed: 300–400 chars — sufficient context without padding the prompt.

File: `pieces.go` → `buildExcerpt`, change the `maxChars` constant.
Expected speedup: modest; reduces prefill time slightly.

---

## Option 3 — Tighten output schema

`toneNotes` is free-text; the model writes a full sentence per post, adding output tokens.
Options:
- Remove `toneNotes` from `DraftPost` and the prompt schema
- Replace with a short enum (`"formal" | "casual" | "educational"`)

Files: `types.go` (DraftPost struct), `prompts/content_plan.tmpl` (output schema block).
Expected speedup: ~10–15% fewer output tokens.

---

## Not worth pursuing

- **Model-level streaming**: already handled via SSE; reduces perceived latency, not total time.
- **Switching models**: already on the fastest/cheapest model (Haiku 4.5).
