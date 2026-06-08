DROP INDEX IF EXISTS idx_posts_cloned_from_post_id;
ALTER TABLE posts DROP COLUMN cloned_from_post_id;
