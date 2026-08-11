-- Reverse CON-147 PR4 soft-delete.
--
-- Preflight guard. Dropping deleted_at would silently REVIVE every soft-deleted
-- workspace (leaving members able to enter a workspace that was deleted). Refuse
-- the rollback while any exist, so reviving them is a deliberate act (hard-delete
-- them, or clear deleted_at, first) rather than a migration side effect. bun runs
-- this file as one implicit transaction, so the RAISE rolls back before the DDL
-- below runs.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM tenants WHERE deleted_at IS NOT NULL) THEN
        RAISE EXCEPTION 'CON-147 PR4 down: soft-deleted workspaces exist; dropping deleted_at would silently revive them. Resolve them first.';
    END IF;
END $$;

DROP INDEX IF EXISTS idx_tenants_active;
ALTER TABLE tenants DROP COLUMN deleted_at;
