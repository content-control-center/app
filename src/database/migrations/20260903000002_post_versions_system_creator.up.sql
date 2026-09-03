-- CON-251: allow the 'system' creator on post_versions.
--
-- The post-status-hardening work records a durable "what actually went out"
-- snapshot at submit time (schedule.persist) and at publish time
-- (VerifyExternal), authored as creator='system' to set it apart from user and
-- assistant content edits. The baseline CHECK only permitted ('assistant',
-- 'user'), so every one of those inserts violated the constraint — and because
-- the snapshot is best-effort (errors are logged and swallowed), the failures
-- were silent and the feature recorded nothing. Widen the constraint to admit
-- 'system'.
ALTER TABLE post_versions DROP CONSTRAINT IF EXISTS post_versions_creator_check;
ALTER TABLE post_versions
    ADD CONSTRAINT post_versions_creator_check
        CHECK (creator IN ('assistant', 'user', 'system'));
