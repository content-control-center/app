-- CON-147 (PR1): split identity (accounts) from membership (users).
--
-- Today a `users` row IS an identity: it carries the password and a globally
-- unique email, and belongs to exactly one tenant. CON-147 lets one login span
-- many workspaces, so identity moves to a new `accounts` table and each `users`
-- row becomes a per-(account, workspace) MEMBERSHIP. Because today's data is
-- strictly 1:1 (email is globally unique -> one user per email), the backfill is
-- deterministic: one account per user, with account.id reusing user.id.
--
-- PR1 is behaviour-preserving: every account still has exactly one membership,
-- so login resolves the same tenant it does today. Per-request workspace
-- resolution (the X-Workspace-Id header) and multi-membership arrive in PR2/PR3.

CREATE TABLE accounts (
    id            TEXT        PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    name          TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One account per existing user (email is globally unique today). account.id
-- reuses user.id so the mapping is obvious and the down migration is trivial.
-- Email is copied verbatim (already unique) rather than lowercased, so the new
-- UNIQUE constraint can't collide on case at migration time.
INSERT INTO accounts (id, email, password_hash, name, created_at, updated_at)
SELECT id, email, password_hash, name, created_at, updated_at FROM users;

-- users -> membership: point every user row at its account.
ALTER TABLE users ADD COLUMN account_id TEXT;
UPDATE users SET account_id = id;
ALTER TABLE users ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_account_id_fkey
    FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE;
CREATE INDEX idx_users_account_id ON users (account_id);

-- An account joins a workspace at most once.
ALTER TABLE users ADD CONSTRAINT users_tenant_account_key UNIQUE (tenant_id, account_id);

-- Email + password now live on the account. Drop the globally-unique email
-- constraint (the same email can be a member of several workspaces once an
-- account spans them) but keep the column, denormalised for existing responses.
-- Drop password_hash entirely -- accounts is the sole source of the credential.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
ALTER TABLE users DROP COLUMN password_hash;

-- sessions authenticate an ACCOUNT (identity). user_id + tenant_id stay as the
-- active membership / default workspace and still drive tenantctx in PR1
-- (per-request resolution is PR2), so scoping is unchanged here.
ALTER TABLE sessions ADD COLUMN account_id TEXT;
UPDATE sessions s SET account_id = u.account_id FROM users u WHERE s.user_id = u.id;
DELETE FROM sessions WHERE account_id IS NULL; -- orphaned sessions (no matching user)
ALTER TABLE sessions ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE sessions ADD CONSTRAINT sessions_account_id_fkey
    FOREIGN KEY (account_id) REFERENCES accounts (id) ON DELETE CASCADE;
