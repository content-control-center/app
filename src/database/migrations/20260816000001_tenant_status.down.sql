-- CON-190 down: drop the tenant lifecycle status. deleted_at (CON-147) is left
-- intact, so soft-deleted tenants remain soft-deleted after a rollback.
DROP INDEX IF EXISTS idx_tenants_status;
ALTER TABLE tenants DROP COLUMN IF EXISTS status_reason;
ALTER TABLE tenants DROP COLUMN IF EXISTS status;
