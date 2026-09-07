-- CON-284: enable the 'thread' post type on Meta Threads. X (Twitter) already
-- lists it (see the platforms seed in 20260622000002_seed_reference_data), but
-- Threads did not — so a thread post targeting Threads would fail the post-type
-- whitelist. Merge the slug into the existing post_types map (jsonb) with `||`
-- so the other entries (text-post, image-post, carousel, video, poll,
-- gif-post) are left untouched.
UPDATE platforms
SET post_types = post_types || '{"thread":"Thread (multi-post sequence)"}'::jsonb
WHERE id = 'pQ4yxT3SuE57';
