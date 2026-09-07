-- CON-156 (BE 6): campaign lifecycle.
--
-- draft vs active is not a distinction the user can act on — nothing behaves
-- differently between the two — so new campaigns are created 'active' (handler
-- default) and the way a campaign leaves the active set becomes archive
-- (reversible) or soft-delete (an operational safety net, not an undo — there
-- is no self-serve restore), NOT status. Existing 'draft' rows are treated as
-- active in code, so no status backfill is needed here.
--
-- archived_at / deleted_at are plain nullable timestamps filtered EXPLICITLY in
-- the repository (mirrors the tenants soft-delete precedent) rather than via
-- bun's `,soft_delete` tag: that keeps archive and delete independent and lets
-- GetByID still return an archived campaign (to unarchive it) while hiding a
-- deleted one. The partial index backs the default active-set list.
ALTER TABLE campaigns
    ADD COLUMN archived_at TIMESTAMPTZ,
    ADD COLUMN deleted_at  TIMESTAMPTZ;

CREATE INDEX idx_campaigns_active ON campaigns (created_at)
    WHERE deleted_at IS NULL AND archived_at IS NULL;
