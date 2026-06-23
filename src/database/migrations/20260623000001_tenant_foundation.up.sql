-- CON-97: multi-tenancy foundation (PR1).
--
-- Introduces the tenants table — the top-level isolation boundary — and a
-- single "default" tenant. This step is purely additive: no existing table is
-- modified. The follow-up migration adds tenant_id columns and backfills all
-- pre-existing rows onto the default tenant.
--
-- Third-party dependency credentials (the secret table) are deliberately NOT
-- tenant-scoped — one Ogen-wide, KEK-encrypted set serves every tenant
-- (CON-97 §10.3) — so secret is untouched here and in the follow-ups.
CREATE TABLE tenants (
    id         TEXT        PRIMARY KEY,
    name       TEXT        NOT NULL,
    slug       TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The default tenant. Every row that predates multi-tenancy is migrated onto
-- this tenant by the migration that introduces the tenant_id columns.
INSERT INTO tenants (id, name, slug) VALUES ('default', 'Default', 'default');
