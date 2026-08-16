-- CON-190: explicit tenant lifecycle status, managed by Harbor over the internal
-- gRPC surface (src/grpcserver, TenantAdminService).
--
-- 'active' is normal; 'suspended' is a reversible operator freeze (non-payment,
-- ToS/abuse hold, investigation) that locks the tenant's users out and pauses
-- its background work without destroying data; 'deleted' is the soft-deleted
-- state introduced in CON-147.
--
-- status is the authoritative lifecycle field. The existing deleted_at timestamp
-- (CON-147) is kept as its companion and is always written together with
-- status='deleted', preserving the invariant:
--     status = 'deleted'  <=>  deleted_at IS NOT NULL
-- deleted_at is retained for the "when" of a deletion, the idx_tenants_active
-- partial index, and the slug-retention semantics CON-147 relies on. New
-- "is this tenant live?" checks use status = 'active'.
ALTER TABLE tenants
    ADD COLUMN status        TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'deleted')),
    ADD COLUMN status_reason TEXT NOT NULL DEFAULT '';

-- Backfill: existing soft-deleted tenants become 'deleted'; everything else
-- keeps the 'active' default. Establishes the invariant for pre-existing rows.
UPDATE tenants SET status = 'deleted' WHERE deleted_at IS NOT NULL;

-- Serves the operator ListTenants status filter and the background-job
-- active-tenant sweeps. The hot auth path is a single-row PK lookup, so the PK
-- index already covers its status post-filter.
CREATE INDEX idx_tenants_status ON tenants (status);
