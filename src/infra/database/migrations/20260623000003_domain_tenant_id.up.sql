-- CON-97 (PR3): tenant_id on every tenant-owned table + composite keys.
--
-- Adds a NOT NULL tenant_id (FK -> tenants, indexed) to all domain tables,
-- backfilling existing rows onto the default tenant. The settings and
-- auto_publish_allowlist primary keys become composite so the same key /
-- platform can exist per tenant. Reference data (platforms, campaigns_types,
-- campaigns_types_phases) and the Ogen-wide secret table are deliberately NOT
-- scoped (CON-97 §5.2 / §10.3).
--
-- Application-level scoping (the TenantScoped model hooks) enforces isolation;
-- these columns + FKs are the storage + integrity backing.

-- ── Simple add-column tables ────────────────────────────────────────────────
ALTER TABLE campaigns               ADD COLUMN tenant_id TEXT;
ALTER TABLE posts                   ADD COLUMN tenant_id TEXT;
ALTER TABLE post_versions           ADD COLUMN tenant_id TEXT;
ALTER TABLE post_assistant_messages ADD COLUMN tenant_id TEXT;
ALTER TABLE post_attachments        ADD COLUMN tenant_id TEXT;
ALTER TABLE post_logs               ADD COLUMN tenant_id TEXT;
ALTER TABLE post_evaluations        ADD COLUMN tenant_id TEXT;
ALTER TABLE post_analytics          ADD COLUMN tenant_id TEXT;
ALTER TABLE assets                  ADD COLUMN tenant_id TEXT;
ALTER TABLE assets_chunks           ADD COLUMN tenant_id TEXT;
ALTER TABLE asset_files             ADD COLUMN tenant_id TEXT;
ALTER TABLE tags                    ADD COLUMN tenant_id TEXT;
ALTER TABLE social_accounts         ADD COLUMN tenant_id TEXT;
ALTER TABLE settings                ADD COLUMN tenant_id TEXT;
ALTER TABLE auto_publish_allowlist  ADD COLUMN tenant_id TEXT;

UPDATE campaigns               SET tenant_id = 'default';
UPDATE posts                   SET tenant_id = 'default';
UPDATE post_versions           SET tenant_id = 'default';
UPDATE post_assistant_messages SET tenant_id = 'default';
UPDATE post_attachments        SET tenant_id = 'default';
UPDATE post_logs               SET tenant_id = 'default';
UPDATE post_evaluations        SET tenant_id = 'default';
UPDATE post_analytics          SET tenant_id = 'default';
UPDATE assets                  SET tenant_id = 'default';
UPDATE assets_chunks           SET tenant_id = 'default';
UPDATE asset_files             SET tenant_id = 'default';
UPDATE tags                    SET tenant_id = 'default';
UPDATE social_accounts         SET tenant_id = 'default';
UPDATE settings                SET tenant_id = 'default';
UPDATE auto_publish_allowlist  SET tenant_id = 'default';

ALTER TABLE campaigns               ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE posts                   ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE post_versions           ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE post_assistant_messages ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE post_attachments        ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE post_logs               ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE post_evaluations        ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE post_analytics          ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE assets                  ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE assets_chunks           ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE asset_files             ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE tags                    ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE social_accounts         ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE settings                ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE auto_publish_allowlist  ALTER COLUMN tenant_id SET NOT NULL;

ALTER TABLE campaigns               ADD CONSTRAINT campaigns_tenant_id_fkey               FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE posts                   ADD CONSTRAINT posts_tenant_id_fkey                   FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE post_versions           ADD CONSTRAINT post_versions_tenant_id_fkey           FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE post_assistant_messages ADD CONSTRAINT post_assistant_messages_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE post_attachments        ADD CONSTRAINT post_attachments_tenant_id_fkey        FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE post_logs               ADD CONSTRAINT post_logs_tenant_id_fkey               FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE post_evaluations        ADD CONSTRAINT post_evaluations_tenant_id_fkey        FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE post_analytics          ADD CONSTRAINT post_analytics_tenant_id_fkey          FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE assets                  ADD CONSTRAINT assets_tenant_id_fkey                  FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE assets_chunks           ADD CONSTRAINT assets_chunks_tenant_id_fkey           FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE asset_files             ADD CONSTRAINT asset_files_tenant_id_fkey             FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE tags                    ADD CONSTRAINT tags_tenant_id_fkey                     FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE social_accounts         ADD CONSTRAINT social_accounts_tenant_id_fkey         FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE settings                ADD CONSTRAINT settings_tenant_id_fkey                FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE auto_publish_allowlist  ADD CONSTRAINT auto_publish_allowlist_tenant_id_fkey  FOREIGN KEY (tenant_id) REFERENCES tenants (id);

CREATE INDEX idx_campaigns_tenant_id               ON campaigns               (tenant_id);
CREATE INDEX idx_posts_tenant_id                   ON posts                   (tenant_id);
CREATE INDEX idx_post_versions_tenant_id           ON post_versions           (tenant_id);
CREATE INDEX idx_post_assistant_messages_tenant_id ON post_assistant_messages (tenant_id);
CREATE INDEX idx_post_attachments_tenant_id        ON post_attachments        (tenant_id);
CREATE INDEX idx_post_logs_tenant_id               ON post_logs               (tenant_id);
CREATE INDEX idx_post_evaluations_tenant_id        ON post_evaluations        (tenant_id);
CREATE INDEX idx_post_analytics_tenant_id          ON post_analytics          (tenant_id);
CREATE INDEX idx_assets_tenant_id                  ON assets                  (tenant_id);
CREATE INDEX idx_assets_chunks_tenant_id           ON assets_chunks           (tenant_id);
CREATE INDEX idx_asset_files_tenant_id             ON asset_files             (tenant_id);
CREATE INDEX idx_tags_tenant_id                    ON tags                    (tenant_id);
CREATE INDEX idx_social_accounts_tenant_id         ON social_accounts         (tenant_id);

-- ── Composite-key tables ────────────────────────────────────────────────────
-- settings: (key) -> (tenant_id, key) so each tenant has its own KV namespace.
ALTER TABLE settings DROP CONSTRAINT settings_pkey;
ALTER TABLE settings ADD PRIMARY KEY (tenant_id, key);

-- auto_publish_allowlist: (platform_id) -> (tenant_id, platform_id).
ALTER TABLE auto_publish_allowlist DROP CONSTRAINT auto_publish_allowlist_pkey;
ALTER TABLE auto_publish_allowlist ADD PRIMARY KEY (tenant_id, platform_id);
