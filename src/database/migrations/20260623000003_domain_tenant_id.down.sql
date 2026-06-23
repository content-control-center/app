-- CON-97 (PR3): reverse domain tenant_id.

-- Restore single-column primary keys.
ALTER TABLE auto_publish_allowlist DROP CONSTRAINT auto_publish_allowlist_pkey;
ALTER TABLE auto_publish_allowlist ADD PRIMARY KEY (platform_id);
ALTER TABLE settings DROP CONSTRAINT settings_pkey;
ALTER TABLE settings ADD PRIMARY KEY (key);

-- Drop FKs, indexes, and columns.
ALTER TABLE campaigns               DROP CONSTRAINT IF EXISTS campaigns_tenant_id_fkey;
ALTER TABLE posts                   DROP CONSTRAINT IF EXISTS posts_tenant_id_fkey;
ALTER TABLE post_versions           DROP CONSTRAINT IF EXISTS post_versions_tenant_id_fkey;
ALTER TABLE post_assistant_messages DROP CONSTRAINT IF EXISTS post_assistant_messages_tenant_id_fkey;
ALTER TABLE post_attachments        DROP CONSTRAINT IF EXISTS post_attachments_tenant_id_fkey;
ALTER TABLE post_logs               DROP CONSTRAINT IF EXISTS post_logs_tenant_id_fkey;
ALTER TABLE post_evaluations        DROP CONSTRAINT IF EXISTS post_evaluations_tenant_id_fkey;
ALTER TABLE post_analytics          DROP CONSTRAINT IF EXISTS post_analytics_tenant_id_fkey;
ALTER TABLE assets                  DROP CONSTRAINT IF EXISTS assets_tenant_id_fkey;
ALTER TABLE assets_chunks           DROP CONSTRAINT IF EXISTS assets_chunks_tenant_id_fkey;
ALTER TABLE asset_files             DROP CONSTRAINT IF EXISTS asset_files_tenant_id_fkey;
ALTER TABLE tags                    DROP CONSTRAINT IF EXISTS tags_tenant_id_fkey;
ALTER TABLE social_accounts         DROP CONSTRAINT IF EXISTS social_accounts_tenant_id_fkey;
ALTER TABLE settings                DROP CONSTRAINT IF EXISTS settings_tenant_id_fkey;
ALTER TABLE auto_publish_allowlist  DROP CONSTRAINT IF EXISTS auto_publish_allowlist_tenant_id_fkey;

ALTER TABLE campaigns               DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE posts                   DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE post_versions           DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE post_assistant_messages DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE post_attachments        DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE post_logs               DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE post_evaluations        DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE post_analytics          DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE assets                  DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE assets_chunks           DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE asset_files             DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE tags                    DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE social_accounts         DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE settings                DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE auto_publish_allowlist  DROP COLUMN IF EXISTS tenant_id;
