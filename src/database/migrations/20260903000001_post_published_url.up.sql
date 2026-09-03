-- CON-165: persist the platform permalink as a first-class field on the post.
-- Until now the published URL survived only inside published_results JSON (the
-- auto-publish path) or in platform_analytics[].platform_post_url in the
-- isolated analytics DB (the verify-external path). Neither is a clean field the
-- front-end can read off a post it already has, forcing an N+1 of analytics
-- calls just to render a "View post" link (CON-149).
--
-- Nullable and backfill-tolerant: a row carries NULL until a publish/verify sets
-- it, or the user pastes one via PUT /api/posts/:id (the Zernio skip path). The
-- "verified" distinction is not a new column — publisher_post_id <> '' already
-- signals a publisher-confirmed post; a URL present with an empty
-- publisher_post_id is a user-supplied (unverified) link.
ALTER TABLE posts
    ADD COLUMN published_url TEXT;

-- Backfill the auto-publish tail from published_results — the first platform
-- outcome's platformPostUrl (posts map to a single platform, so element 0 is the
-- one). Same-DB, so this is a plain UPDATE. The verify-external tail (URL only in
-- the separate analytics DB) is not reachable from here and is left to an
-- app-level backfill; those posts also self-heal on the next verify/refresh.
UPDATE posts
SET published_url = published_results::jsonb -> 0 ->> 'platformPostUrl'
WHERE published_url IS NULL
  AND published_results <> ''
  AND jsonb_typeof(published_results::jsonb) = 'array'
  AND NULLIF(published_results::jsonb -> 0 ->> 'platformPostUrl', '') IS NOT NULL;
