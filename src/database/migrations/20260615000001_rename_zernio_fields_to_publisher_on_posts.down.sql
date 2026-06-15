DROP INDEX IF EXISTS idx_posts_publisher_post_id;
ALTER TABLE posts DROP COLUMN publisher;
ALTER TABLE posts RENAME COLUMN publisher_status TO zernio_status;
ALTER TABLE posts RENAME COLUMN publisher_post_id TO zernio_post_id;
CREATE UNIQUE INDEX idx_posts_zernio_post_id ON posts (zernio_post_id) WHERE zernio_post_id IS NOT NULL;
