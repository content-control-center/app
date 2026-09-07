-- CON-284: threaded messages. A post marked platform_post_type = 'thread'
-- publishes as an ordered chain (root + replies) on X or Threads instead of a
-- single message. Two additive columns carry the structure; no new table
-- (per-segment referential integrity for media is expressed by segment_index,
-- validated in the app against the segment count, rather than an FK).

-- thread_segments is an ordered jsonb array of {"content": "..."} objects:
-- index 0 is the root message, 1..N-1 the ordered replies. Empty [] for every
-- non-thread post (today's behaviour). posts.content mirrors segment 0 so the
-- many readers of post.content (quality, listings, campaign summaries,
-- analytics title) keep working without thread-awareness.
ALTER TABLE posts
    ADD COLUMN thread_segments jsonb NOT NULL DEFAULT '[]';

-- segment_index names which thread segment an attachment belongs to (0-based
-- into thread_segments). NULL for every attachment of a non-thread post — the
-- attachment belongs to the whole/single post, today's behaviour. Within a
-- segment, position still orders the media, so the existing
-- UNIQUE (post_id, position) is untouched (positions stay post-global).
ALTER TABLE post_attachments
    ADD COLUMN segment_index int;
