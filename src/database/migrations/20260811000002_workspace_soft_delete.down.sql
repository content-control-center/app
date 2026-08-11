-- Reverse CON-147 PR4 soft-delete. Any workspace currently soft-deleted becomes
-- live again (the column and its filter are gone), which is the only sane
-- reversal — the down migration can't invent a hard delete.
DROP INDEX IF EXISTS idx_tenants_active;
ALTER TABLE tenants DROP COLUMN deleted_at;
