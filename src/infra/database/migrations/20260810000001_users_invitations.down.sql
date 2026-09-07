-- CON-26: drop the invitations store and the users.role column (its CHECK
-- constraint and indexes are dropped with them).
DROP TABLE IF EXISTS users_invitations;
ALTER TABLE users DROP COLUMN IF EXISTS role;
