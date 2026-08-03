-- CON-154: transactional & marketing email subsystem.

-- Editable template store; seeded on boot from embedded Maizzle-compiled
-- defaults (upsert-if-absent, DB wins thereafter). One row per template key.
CREATE TABLE email_templates (
    key        TEXT        PRIMARY KEY,
    subject    TEXT        NOT NULL,
    html       TEXT        NOT NULL,
    text       TEXT        NOT NULL,
    kind       TEXT        NOT NULL CHECK (kind IN ('transactional', 'marketing')),
    version    INTEGER     NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Append-only send audit (mirrors post_logs). tenant_id / user_id are nullable
-- for mail with no owning tenant/user; a deleted user keeps its log rows
-- (ON DELETE SET NULL). Delivery webhooks update rows in place by
-- provider_message_id.
CREATE TABLE email_logs (
    id                  TEXT        PRIMARY KEY,
    tenant_id           TEXT        REFERENCES tenants (id),
    user_id             TEXT        REFERENCES users (id) ON DELETE SET NULL,
    template_id         TEXT        NOT NULL,
    kind                TEXT        NOT NULL CHECK (kind IN ('transactional', 'marketing')),
    to_email            TEXT        NOT NULL,
    status              TEXT        NOT NULL,
    provider            TEXT        NOT NULL DEFAULT 'resend',
    provider_message_id TEXT,
    idempotency_key     TEXT,
    error               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_email_logs_provider_message_id ON email_logs (provider_message_id);
CREATE INDEX idx_email_logs_to_email ON email_logs (to_email);
CREATE INDEX idx_email_logs_tenant_created ON email_logs (tenant_id, created_at);
-- Idempotency backstop: only rows that actually carry a key are constrained,
-- so keyless rows (never expected in v1) can't collide on an empty string.
CREATE UNIQUE INDEX idx_email_logs_idempotency_key
    ON email_logs (idempotency_key) WHERE idempotency_key IS NOT NULL;

-- Suppression list: who not to mail. Keyed by email address (the global
-- identifier) + scope, so an unsubscribe (marketing) and a hard bounce (all)
-- coexist as distinct rows.
CREATE TABLE email_suppressions (
    id         TEXT        PRIMARY KEY,
    email      TEXT        NOT NULL,
    scope      TEXT        NOT NULL CHECK (scope IN ('marketing', 'all')),
    reason     TEXT        NOT NULL,
    source     TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (email, scope)
);
CREATE INDEX idx_email_suppressions_email ON email_suppressions (email);
