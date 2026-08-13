-- CON-147 (PR4): soft-delete for workspaces.
--
-- A deleted workspace keeps its row — an operational safety net, not an undo:
-- there is no self-serve restore, so recovery is a manual support operation. But
-- it leaves every member's list and can't be entered again, because membership
-- resolution filters deleted_at (a deleted workspace becomes indistinguishable
-- from one the account was never in). Published posts stay live on the networks;
-- hard-deleting content and tearing down the workspace's Zernio profile can
-- follow later without touching the client.
--
-- Kept deliberately NOT a bun `,soft_delete` column: that would auto-exclude
-- deleted tenants from GetBySlug too, freeing a slug the (surviving) unique row
-- still holds and breaking the next create. Filtering is explicit instead, only
-- at the membership-resolution boundary.

ALTER TABLE tenants ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX idx_tenants_active ON tenants (id) WHERE deleted_at IS NULL;
