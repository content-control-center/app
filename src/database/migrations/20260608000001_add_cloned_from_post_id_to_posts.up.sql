-- CON-59: a cloned Post links back to its source for lineage and a
-- future "duplicated from" affordance. Nullable — posts created
-- directly have no source. Partial index keeps the common
-- (source IS NULL) rows out of the index.
ALTER TABLE posts ADD COLUMN cloned_from_post_id TEXT;
CREATE INDEX idx_posts_cloned_from_post_id ON posts (cloned_from_post_id) WHERE cloned_from_post_id IS NOT NULL;
