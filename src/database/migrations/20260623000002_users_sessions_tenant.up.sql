-- CON-97 (PR2): attach users and sessions to tenants.
--
-- Every user belongs to exactly one tenant; sessions carry a denormalised
-- tenant_id so the auth middleware resolves the tenant without a second query.
-- All pre-existing rows are backfilled onto the default tenant seeded by the
-- foundation migration. Email stays GLOBALLY unique (the existing constraint
-- is untouched): login is by email -> user -> tenant.

ALTER TABLE users ADD COLUMN tenant_id TEXT;
UPDATE users SET tenant_id = 'default';
ALTER TABLE users ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE users ADD CONSTRAINT users_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants (id);
CREATE INDEX idx_users_tenant_id ON users (tenant_id);

ALTER TABLE sessions ADD COLUMN tenant_id TEXT;
UPDATE sessions s SET tenant_id = u.tenant_id FROM users u WHERE s.user_id = u.id;
-- Orphan sessions (no matching user) fall back to the default tenant.
UPDATE sessions SET tenant_id = 'default' WHERE tenant_id IS NULL;
ALTER TABLE sessions ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE sessions ADD CONSTRAINT sessions_tenant_id_fkey
    FOREIGN KEY (tenant_id) REFERENCES tenants (id);
