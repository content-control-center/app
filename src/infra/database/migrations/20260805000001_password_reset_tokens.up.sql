-- CON-161: single-use, hashed password-reset tokens. Only the sha256 hash of a
-- token is stored (token_hash) — a DB leak must not hand over live reset links.
-- consumed_at NULL means unspent; the confirm path flips it atomically, so a
-- token is single-use even under a double-submitted form. Expired rows are never
-- accepted; they are pruned out-of-band, not on the request path.
CREATE TABLE password_reset_tokens (
    id          TEXT        PRIMARY KEY,
    user_id     TEXT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    tenant_id   TEXT        NOT NULL,
    token_hash  TEXT        NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_password_reset_tokens_user_id ON password_reset_tokens (user_id);
