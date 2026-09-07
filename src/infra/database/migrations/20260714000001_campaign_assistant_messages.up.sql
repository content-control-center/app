-- CON-112: per-campaign conversation history between the user and the
-- Campaign Assistant. Mirrors post_assistant_messages, tenant-scoped from the
-- start (CON-97) since it is created after the tenant foundation migrations.
CREATE TABLE campaign_assistant_messages (
    id          TEXT        PRIMARY KEY,
    campaign_id TEXT        NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    role        TEXT        NOT NULL CHECK (role IN ('user', 'model')),
    content     TEXT        NOT NULL,
    tenant_id   TEXT        NOT NULL REFERENCES tenants (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_campaign_assistant_messages_campaign_id ON campaign_assistant_messages (campaign_id);
CREATE INDEX idx_campaign_assistant_messages_tenant_id ON campaign_assistant_messages (tenant_id);
