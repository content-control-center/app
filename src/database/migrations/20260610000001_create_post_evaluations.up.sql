-- post_evaluations holds the latest Post quality assessment (CON-85).
-- Exactly one row per Post — UNIQUE(post_id) — so re-evaluating a Post
-- OVERWRITES the previous result via upsert rather than versioning;
-- evaluation history is an explicitly deferred concern in the spec.
-- FK CASCADE drops the evaluation when its Post is deleted.
--
-- The four per-dimension blocks (Correctness, Clarity, Engagement,
-- Delivery) — each carrying the model's 0-10 score, the rationale it
-- wrote before the score, at least one named weakness, the span-anchored
-- suggestions, and the backend-computed weight + weighted contribution —
-- are stored together in the `result` JSON column. The model returns the
-- sub-scores and critique; the backend composes overall_pct and the
-- per-dimension contributions deterministically (it is never returned by
-- the model).
--
-- overall_pct is denormalised into its own column because it is the
-- headline number list/sort views read without parsing JSON.
-- caption_scoped flags that the score saw only caption/text — the visual
-- of an image/carousel post is not judged in v1.
CREATE TABLE post_evaluations
(
    id                 TEXT     PRIMARY KEY,
    post_id            TEXT     NOT NULL UNIQUE REFERENCES posts (id) ON DELETE CASCADE,
    platform_id        TEXT     NOT NULL DEFAULT '',
    platform_post_type TEXT     NOT NULL DEFAULT '',
    caption_scoped     INTEGER  NOT NULL DEFAULT 0,
    overall_pct        REAL     NOT NULL DEFAULT 0,
    result             TEXT     NOT NULL DEFAULT '{}',
    model_id           TEXT     NOT NULL DEFAULT '',
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
