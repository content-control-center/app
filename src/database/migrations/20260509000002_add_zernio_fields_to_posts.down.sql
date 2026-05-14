DROP INDEX IF EXISTS idx_posts_zernio_post_id;
ALTER TABLE posts DROP COLUMN failure_reason;
ALTER TABLE posts DROP COLUMN published_results;
ALTER TABLE posts DROP COLUMN zernio_status;
ALTER TABLE posts DROP COLUMN zernio_post_id;
