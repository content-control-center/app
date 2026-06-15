-- CON-93 §14 prerequisite: generalize the CON-69 Zernio-specific post
-- columns to publisher-agnostic names so analytics (and future
-- publishers) key off `publisher` + `publisher_post_id` rather than a
-- hard-coded Zernio identity. Only Ogen's DB/API surface is renamed
-- here — the Zernio *adapter* internals (queue names, zernio.* settings,
-- client methods) stay Zernio-specific.
--
-- `publisher` marks which publisher adapter owns the external identity
-- (matches publishers.Publisher.ID(), e.g. 'zernio'). It is backfilled
-- to 'zernio' for every row that already carries a publisher post id
-- from the CON-69 pipeline; rows never published through a publisher
-- keep the empty-string default. The UNIQUE partial index that guarded
-- against double-submit is recreated under the new column name.
DROP INDEX IF EXISTS idx_posts_zernio_post_id;
ALTER TABLE posts RENAME COLUMN zernio_post_id TO publisher_post_id;
ALTER TABLE posts RENAME COLUMN zernio_status TO publisher_status;
ALTER TABLE posts ADD COLUMN publisher TEXT NOT NULL DEFAULT '';
UPDATE posts SET publisher = 'zernio' WHERE publisher_post_id IS NOT NULL AND publisher_post_id <> '';
CREATE UNIQUE INDEX idx_posts_publisher_post_id ON posts (publisher_post_id) WHERE publisher_post_id IS NOT NULL;
