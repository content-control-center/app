-- CON-97 (PR2): detach users and sessions from tenants.
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_tenant_id_fkey;
ALTER TABLE sessions DROP COLUMN IF EXISTS tenant_id;
DROP INDEX IF EXISTS idx_users_tenant_id;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_tenant_id_fkey;
ALTER TABLE users DROP COLUMN IF EXISTS tenant_id;
