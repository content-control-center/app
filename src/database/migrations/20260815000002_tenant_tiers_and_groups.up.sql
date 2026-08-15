-- CON-208: operator-owned tenant classification — a required tier (1 per tenant)
-- and optional groups (many-to-many). Managed by Harbor over the internal gRPC
-- surface (src/grpcserver). This migration creates the schema, seeds a default
-- tier, and backfills existing tenants.
--
-- These three tables are GLOBAL operator tables (NOT tenant-scoped) — like
-- tenants/accounts — so they are read and written cross-tenant and carry no
-- tenant_id predicate.

-- Tier catalog. Shared across tenants (few rows, e.g. Free/Pro/Enterprise).
-- color: optional hex label ('' = none). description: optional free text.
CREATE TABLE tenant_tiers (
    id          TEXT        PRIMARY KEY,
    name        TEXT        NOT NULL UNIQUE,
    color       TEXT        NOT NULL DEFAULT '',
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Group catalog. Shared, reusable cohorts/segments.
CREATE TABLE tenant_groups (
    id          TEXT        PRIMARY KEY,
    name        TEXT        NOT NULL UNIQUE,
    color       TEXT        NOT NULL DEFAULT '',
    description TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seed the default tier BEFORE the tenants.tier_id backfill needs it. It is the
-- signup fallback and is delete-protected in the service layer, so it always
-- exists.
INSERT INTO tenant_tiers (id, name) VALUES ('default', 'Default');

-- Each tenant has exactly one tier. Backfill to default, then enforce NOT NULL.
-- ON DELETE RESTRICT: a tier still assigned to a tenant cannot be deleted
-- (Harbor must reassign first) — tier is required, so it must never be orphaned.
ALTER TABLE tenants ADD COLUMN tier_id TEXT
    REFERENCES tenant_tiers (id) ON DELETE RESTRICT;
UPDATE tenants SET tier_id = 'default';
ALTER TABLE tenants ALTER COLUMN tier_id SET NOT NULL;
CREATE INDEX idx_tenants_tier_id ON tenants (tier_id);

-- Many-to-many: a tenant belongs to many groups, a group holds many tenants.
CREATE TABLE tenant_group_assignments (
    tenant_id  TEXT        NOT NULL REFERENCES tenants (id)       ON DELETE CASCADE,
    group_id   TEXT        NOT NULL REFERENCES tenant_groups (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, group_id)
);
-- Reverse "who is in this group" lookups. (tenant_id lookups are served by the
-- composite PK's leading column.)
CREATE INDEX idx_tenant_group_assignments_group_id ON tenant_group_assignments (group_id);
