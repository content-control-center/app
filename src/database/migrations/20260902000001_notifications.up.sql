-- CON-242: notification center. A persistent, per-user in-app notification
-- inbox — distinct from the ephemeral eventhub bus (which loses everything on
-- disconnect) and the email channel (out-of-app). One row per recipient
-- (fan-out on write), so read/unread is a plain column and a read is a trivial
-- `WHERE user_id = me`.
--
-- `seq` is a monotonic BIGSERIAL used as the SSE stream cursor (Last-Event-ID /
-- ?since=): the sqids `id` is cryptographically random, so it can't order the
-- stream.
--
-- `level` is a stable, closed severity set, so it carries a CHECK. `type` is a
-- free-form machine key (post.publish_failed, connection.expiring_soon, …),
-- expected to grow, so it is validated in Go (no CHECK) — mirroring
-- post_notes.type. Tenant-scoped from the start (CON-97).
CREATE TABLE notifications (
    id           TEXT        PRIMARY KEY,
    seq          BIGSERIAL   NOT NULL,
    tenant_id    TEXT        NOT NULL REFERENCES tenants (id),
    user_id      TEXT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    level        TEXT        NOT NULL DEFAULT 'info'
                             CHECK (level IN ('info', 'success', 'warning', 'error')),
    type         TEXT        NOT NULL,
    title        TEXT        NOT NULL,
    body         TEXT        NOT NULL DEFAULT '',
    entity_type  TEXT        NOT NULL DEFAULT '',
    entity_id    TEXT        NOT NULL DEFAULT '',
    action_url   TEXT        NOT NULL DEFAULT '',
    data         JSONB,
    dedupe_key   TEXT,
    read_at      TIMESTAMPTZ,
    dismissed_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Inbox read + keyset pagination: newest-first per user (tenant filtered by the
-- app-level scope hook).
CREATE INDEX idx_notifications_user_seq ON notifications (user_id, seq DESC);

-- Unread-count + unread filter: a partial index over just the live, unread rows.
CREATE INDEX idx_notifications_user_unread ON notifications (user_id)
    WHERE read_at IS NULL AND dismissed_at IS NULL;

CREATE INDEX idx_notifications_tenant ON notifications (tenant_id);

-- Collapse duplicates for the same user while still unread (e.g. a re-swept
-- connection-expiry). Producers set dedupe_key; inserts use ON CONFLICT DO
-- NOTHING. A NULL dedupe_key never collapses (partial predicate).
CREATE UNIQUE INDEX idx_notifications_dedupe ON notifications (user_id, dedupe_key)
    WHERE dedupe_key IS NOT NULL AND read_at IS NULL;

-- Retention/expiry sweep target (cleanup_notifications).
CREATE INDEX idx_notifications_expires ON notifications (expires_at)
    WHERE expires_at IS NOT NULL;
