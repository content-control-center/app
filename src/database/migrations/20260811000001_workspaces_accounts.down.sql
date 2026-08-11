-- Reverse CON-147 PR1: fold accounts back into users.
--
-- Restores the pre-CON-147 shape: users regains password_hash + the globally
-- unique email, sessions loses account_id, and the accounts table is dropped.
-- Deterministic because the split was 1:1 (account.id == user.id).

-- Preflight guard. Folding accounts back into users is only reversible while the
-- split is still 1:1. Once an account holds several memberships (the many-to-many
-- state CON-147 enables), restoring the globally-unique users.email would collide
-- and dropping accounts would lose those memberships. Abort the whole rollback if
-- any such account exists — bun runs this file as one implicit transaction, so
-- the RAISE rolls back before any statement below runs.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users GROUP BY account_id HAVING count(*) > 1) THEN
        RAISE EXCEPTION 'CON-147 down: an account has multiple workspace memberships; refusing to fold back to one-user-per-account (would violate users.email UNIQUE and drop memberships). Reduce every account to one membership first.';
    END IF;
END $$;

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
