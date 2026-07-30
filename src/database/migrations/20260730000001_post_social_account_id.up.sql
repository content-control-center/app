-- CON-150: let a post name which same-platform account it publishes to.
-- Nullable so drafts/legacy rows (and posts on platforms with a single
-- connected account) stay NULL and resolve at submit time. social_accounts
-- rows are soft-deleted (never hard-deleted), so the FK never breaks; ON
-- DELETE SET NULL is defensive in case a row is ever purged.
ALTER TABLE posts
    ADD COLUMN social_account_id TEXT
    REFERENCES social_accounts (id) ON DELETE SET NULL;
