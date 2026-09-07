-- CON-284 rollback: drop the threaded-messages columns.
ALTER TABLE post_attachments
    DROP COLUMN segment_index;

ALTER TABLE posts
    DROP COLUMN thread_segments;
