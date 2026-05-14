-- CON-69 §6: posts now carry the Zernio job they belong to and the
-- per-platform publish outcomes Zernio reports back. zernio_post_id
-- is UNIQUE so a retry storm cannot double-submit (CON-69 §12 — the
-- 24h dedupe path stores the existing job id here on recovery).
-- zernio_status mirrors Zernio's own enum verbatim so the polling
-- task can short-circuit on terminal-but-already-handled states.
-- published_results is the per-platform JSON ({platform, status,
-- platformPostUrl, error?}) Zernio returns once a post resolves.
ALTER TABLE posts ADD COLUMN zernio_post_id TEXT;
CREATE UNIQUE INDEX idx_posts_zernio_post_id ON posts (zernio_post_id) WHERE zernio_post_id IS NOT NULL;
ALTER TABLE posts ADD COLUMN zernio_status TEXT NOT NULL DEFAULT '';
ALTER TABLE posts ADD COLUMN published_results TEXT NOT NULL DEFAULT '';
ALTER TABLE posts ADD COLUMN failure_reason TEXT NOT NULL DEFAULT '';
