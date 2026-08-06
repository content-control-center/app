-- CON-188: per-post Notes. A note is a small standalone record (draft thesis,
-- image prompt, or free-form note) attached to a post, so ancillary content
-- lives here instead of in the post body. Tenant-scoped from the start (CON-97).
--
-- `type` has no CHECK constraint on purpose: the vocabulary is expected to grow
-- and is validated in Go (models.PostNoteType). `origin` is a stable, closed
-- set, so it carries a CHECK.
CREATE TABLE post_notes (
    id         TEXT        PRIMARY KEY,
    post_id    TEXT        NOT NULL REFERENCES posts (id) ON DELETE CASCADE,
    type       TEXT        NOT NULL,
    title      TEXT        NOT NULL DEFAULT '',
    body       TEXT        NOT NULL,
    origin     TEXT        NOT NULL DEFAULT 'manual'
                             CHECK (origin IN ('manual', 'assistant', 'content_plan')),
    tenant_id  TEXT        NOT NULL REFERENCES tenants (id),
    created_by TEXT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_post_notes_post_id   ON post_notes (post_id);
CREATE INDEX idx_post_notes_tenant_id ON post_notes (tenant_id);
