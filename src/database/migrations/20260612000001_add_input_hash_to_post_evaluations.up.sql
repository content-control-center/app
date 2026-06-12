-- input_hash fingerprints the assessment inputs (the rendered system+user
-- prompt plus the model id) for the stored evaluation (CON-92). Before
-- calling the model, the assess flow recomputes this hash and skips the
-- expensive model run when it matches — i.e. it re-assesses only when
-- something the model actually sees has changed. Defaults to '' so existing
-- rows are treated as "unknown inputs" and re-assess once on next request.
ALTER TABLE post_evaluations ADD COLUMN input_hash TEXT NOT NULL DEFAULT '';
