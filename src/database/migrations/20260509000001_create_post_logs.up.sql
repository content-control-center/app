-- post_logs is the auditable, queryable history of every meaningful
-- operation against a Post (CON-69 §11). Every state transition,
-- validation result, allowlist decision, background-task lifecycle
-- event, Zernio API interaction, reconciliation timeout, and user
-- action lands here. Together with FK CASCADE this lets us reconstruct
-- exactly what happened to a Post — and to ops/debug across Posts —
-- without diverging from the actual Post row state (writes are made
-- in the same transaction wherever possible).
--
-- payload is a TEXT column holding sanitized JSON capped at 64 KB by
-- the writer (postlog.Capper). Larger payloads get truncated with a
-- "…[truncated, original=N bytes]" marker so the column never bloats
-- the SQLite file. Sensitive fields (API keys, full credentials) are
-- never written; redaction happens in postlog.Sanitize before insert.
CREATE TABLE post_logs
(
    id              TEXT     PRIMARY KEY,
    post_id         TEXT     NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    event_timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_type      TEXT     NOT NULL,
    actor           TEXT     NOT NULL,
    from_status     TEXT,
    to_status       TEXT,
    payload         TEXT     NOT NULL DEFAULT '{}',
    summary         TEXT     NOT NULL DEFAULT ''
);
CREATE INDEX idx_post_logs_post_id          ON post_logs (post_id);
CREATE INDEX idx_post_logs_event_timestamp  ON post_logs (event_timestamp);
CREATE INDEX idx_post_logs_event_type       ON post_logs (event_type);
