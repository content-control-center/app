-- CON-228: Brand materials storage — voices, audiences, guardrails, look, templates.
--
-- Five tenant-scoped stores backing GET /api/brand and its per-section writes.
-- Isolation is enforced app-side by the TenantScoped bun hooks (CON-97 §6); the
-- tenant_id FK + index here are the storage half of that. Compound sub-objects
-- (rules, origin, palette, ratios, …) are jsonb so a whole-resource write is one
-- row. See the ui repo's docs/brand-materials.md for the argument behind each.

-- ── Voices (library) ────────────────────────────────────────────────────────
CREATE TABLE brand_voices (
    id            TEXT        PRIMARY KEY,
    tenant_id     TEXT        NOT NULL REFERENCES tenants (id),
    name          TEXT        NOT NULL,
    when_to_use   TEXT        NOT NULL DEFAULT '',
    is_default    BOOLEAN     NOT NULL DEFAULT FALSE,
    samples       JSONB       NOT NULL DEFAULT '[]',
    rules         JSONB       NOT NULL DEFAULT '{}',
    channel_notes JSONB       NOT NULL DEFAULT '{}',
    origin        JSONB       NOT NULL DEFAULT '{"kind":"blank"}',
    summary       TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_brand_voices_tenant_id ON brand_voices (tenant_id);
-- FR4: at most one default voice per tenant, enforced in the DB so a racing
-- double-write cannot leave two.
CREATE UNIQUE INDEX idx_brand_voices_one_default ON brand_voices (tenant_id) WHERE is_default;

-- ── Audiences (library) ─────────────────────────────────────────────────────
CREATE TABLE brand_audiences (
    id                TEXT        PRIMARY KEY,
    tenant_id         TEXT        NOT NULL REFERENCES tenants (id),
    name              TEXT        NOT NULL,
    who               TEXT        NOT NULL DEFAULT '',
    reads_on          TEXT        NOT NULL DEFAULT '',
    scrolls_past_when TEXT        NOT NULL DEFAULT '',
    believes_when     TEXT        NOT NULL DEFAULT '',
    origin            JSONB       NOT NULL DEFAULT '{"kind":"blank"}',
    summary           TEXT        NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_brand_audiences_tenant_id ON brand_audiences (tenant_id);

-- ── Guardrails (singleton) ──────────────────────────────────────────────────
CREATE TABLE brand_guardrails (
    id           TEXT        PRIMARY KEY,
    tenant_id    TEXT        NOT NULL REFERENCES tenants (id),
    facts        JSONB       NOT NULL DEFAULT '[]',
    may_claim    JSONB       NOT NULL DEFAULT '[]',
    never_claim  JSONB       NOT NULL DEFAULT '[]',
    banned_words JSONB       NOT NULL DEFAULT '[]',
    disclaimer   TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- One guardrails record per tenant.
CREATE UNIQUE INDEX idx_brand_guardrails_tenant ON brand_guardrails (tenant_id);

-- ── Look (singleton) ────────────────────────────────────────────────────────
CREATE TABLE brand_look (
    id               TEXT        PRIMARY KEY,
    tenant_id        TEXT        NOT NULL REFERENCES tenants (id),
    logos            JSONB       NOT NULL DEFAULT '[]',
    palette          JSONB       NOT NULL DEFAULT '[]',
    typefaces        JSONB       NOT NULL DEFAULT '[]',
    reference_images JSONB       NOT NULL DEFAULT '[]',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_brand_look_tenant ON brand_look (tenant_id);

-- ── Templates (library) ─────────────────────────────────────────────────────
CREATE TABLE brand_templates (
    id         TEXT        PRIMARY KEY,
    tenant_id  TEXT        NOT NULL REFERENCES tenants (id),
    name       TEXT        NOT NULL,
    role       TEXT        NOT NULL DEFAULT 'foreground',
    ratios     JSONB       NOT NULL DEFAULT '[]',
    is_default BOOLEAN     NOT NULL DEFAULT FALSE,
    platforms  JSONB       NOT NULL DEFAULT '[]',
    origin     JSONB       NOT NULL DEFAULT '{"kind":"blank"}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_brand_templates_tenant_id ON brand_templates (tenant_id);
CREATE UNIQUE INDEX idx_brand_templates_one_default ON brand_templates (tenant_id) WHERE is_default;
