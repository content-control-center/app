-- Reverse CON-147 PR1: fold accounts back into users.
--
-- Restores the pre-CON-147 shape: users regains password_hash + the globally
-- unique email, sessions loses account_id, and the accounts table is dropped.
-- Deterministic because the split was 1:1 (account.id == user.id).

ALTER TABLE users ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';
UPDATE users u SET password_hash = a.password_hash FROM accounts a WHERE u.account_id = a.id;

ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_account_id_fkey;
ALTER TABLE sessions DROP COLUMN account_id;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_tenant_account_key;
DROP INDEX IF EXISTS idx_users_account_id;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_account_id_fkey;
ALTER TABLE users DROP COLUMN account_id;
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);

DROP TABLE accounts;
