-- CON-86: per-tenant usage spend caps (control-plane DB). One row per tenant;
-- an absent row falls back to the config global defaults, else unlimited.
CREATE TABLE tenant_usage_limits (
    id                 TEXT        PRIMARY KEY,
    tenant_id          TEXT        NOT NULL REFERENCES tenants (id),
    daily_cap_micros   BIGINT      CHECK (daily_cap_micros   IS NULL OR daily_cap_micros   >= 0),
    monthly_cap_micros BIGINT      CHECK (monthly_cap_micros IS NULL OR monthly_cap_micros >= 0),
    mode               TEXT        NOT NULL DEFAULT 'enforce'
        CHECK (mode IN ('enforce', 'warn')),
    enabled            BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id)
);

CREATE INDEX idx_tenant_usage_limits_tenant_id ON tenant_usage_limits (tenant_id);
