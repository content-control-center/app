-- CON-26: workspace roles + user invitations.
--
-- 1) users.role — a minimal owner/member authority per tenant. New rows default
--    to 'member' (invitation accept / POST /api/users set it explicitly); every
--    pre-CON-26 user is backfilled to 'owner' because each was the sole member of
--    its tenant and thus unambiguously its owner. Forward-compatible with
--    CON-147, which inserts an 'admin' tier between the two.
ALTER TABLE users
    ADD COLUMN role TEXT NOT NULL DEFAULT 'member'
        CHECK (role IN ('owner', 'member'));
UPDATE users SET role = 'owner';

-- 2) users_invitations — single-use, hashed invitations to join a workspace with
--    a preset role. Only the sha256 hash of the token is stored (token_hash) — a
--    DB leak must not hand over live invites. Accepting an invite creates a users
--    row in tenant_id (Ogen stays one-user-one-tenant; the multi-workspace
--    account split is CON-147). Expiry is derived from expires_at, not a stored
--    status; expired-but-pending rows are pruned out-of-band, never accepted.
CREATE TABLE users_invitations (
    id          TEXT        PRIMARY KEY,
    tenant_id   TEXT        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    email       TEXT        NOT NULL,
    role        TEXT        NOT NULL CHECK (role IN ('owner', 'member')),
    token_hash  TEXT        NOT NULL UNIQUE,
    invited_by  TEXT        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status      TEXT        NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'accepted', 'revoked')),
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- At most one live (pending) invite per address per workspace; a revoked or
-- accepted invite doesn't block re-inviting the same address.
CREATE UNIQUE INDEX idx_users_invitations_pending
    ON users_invitations (tenant_id, email)
    WHERE status = 'pending';

-- List a workspace's invitations.
CREATE INDEX idx_users_invitations_tenant_id ON users_invitations (tenant_id);
