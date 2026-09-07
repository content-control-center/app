-- CON-251 down: restore the original creator allow-list. Any 'system' snapshot
-- rows must be reassigned first, otherwise the narrower CHECK would reject the
-- existing data; fold them into 'user' (the closest human-facing bucket) so the
-- rollback applies cleanly.
UPDATE post_versions SET creator = 'user' WHERE creator = 'system';
ALTER TABLE post_versions DROP CONSTRAINT IF EXISTS post_versions_creator_check;
ALTER TABLE post_versions
    ADD CONSTRAINT post_versions_creator_check
        CHECK (creator IN ('assistant', 'user'));
