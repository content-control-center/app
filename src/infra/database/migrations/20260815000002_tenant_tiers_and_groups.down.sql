-- CON-208: drop the tenant classification schema. Order matters — the join and
-- the tenants.tier_id FK must go before the catalogs they reference.
DROP TABLE IF EXISTS tenant_group_assignments;
DROP INDEX  IF EXISTS idx_tenants_tier_id;
ALTER TABLE tenants DROP COLUMN IF EXISTS tier_id;
DROP TABLE IF EXISTS tenant_groups;
DROP TABLE IF EXISTS tenant_tiers;
