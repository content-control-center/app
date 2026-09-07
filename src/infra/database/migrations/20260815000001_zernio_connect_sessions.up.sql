-- CON-217: short-lived, server-side state for headless Zernio account
-- connection (hide Zernio's hosted account selector).
--
-- A row is created when a connect link is issued (status pending_auth); the
-- OAuth callback either finalizes it (row deleted) or transitions it to
-- awaiting_selection when 2+ targets (e.g. several LinkedIn organizations /
-- Facebook pages) need an in-Ogen pick. Rows are deleted on completion; a
-- periodic job sweeps expired ones, and readers also treat expires_at in the
-- past as gone.
--
-- NOT tenant-scoped by a bun hook: the OAuth callback resolves the tenant FROM
-- this row (via the opaque id) because the browser round-trips through Zernio
-- without a session. tenant_id is a plain column, scoped explicitly at the
-- authenticated picker boundary (mirrors sessions, CON-97 PR2). The short-lived
-- Zernio secrets are stored encrypted in sealed_secrets (envelope-encrypted
-- JSON); options holds only non-secret display fields for the picker.
CREATE TABLE zernio_connect_sessions (
    id             TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL,
    profile_id     TEXT NOT NULL,
    platform       TEXT NOT NULL,
    status         TEXT NOT NULL,
    sealed_secrets BYTEA,
    options        JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL
);

-- The expiry sweep scans by expires_at.
CREATE INDEX idx_zernio_connect_sessions_expires_at ON zernio_connect_sessions (expires_at);
