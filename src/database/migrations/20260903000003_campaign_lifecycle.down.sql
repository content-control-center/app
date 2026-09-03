-- Reverse CON-156 BE 6 campaign lifecycle.
--
-- Preflight guard. Dropping deleted_at would silently REVIVE every soft-deleted
-- campaign (returning it to lists and letting it be edited again). Refuse the
-- rollback while any exist, so reviving them is a deliberate act (hard-delete
-- them, or clear deleted_at, first) rather than a migration side effect. bun
-- runs this file as one implicit transaction, so the RAISE rolls back before
-- the DDL below runs. Archived rows carry no such risk — dropping archived_at
-- just returns them to the active set — so they are not guarded.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM campaigns WHERE deleted_at IS NOT NULL) THEN
        RAISE EXCEPTION 'CON-156 BE6 down: soft-deleted campaigns exist; dropping deleted_at would silently revive them. Resolve them first.';
    END IF;
END $$;

DROP INDEX IF EXISTS idx_campaigns_active;
ALTER TABLE campaigns
    DROP COLUMN archived_at,
    DROP COLUMN deleted_at;
