-- Per-platform image attachment constraints (CON-73). Stored on the
-- platform row itself so the rules travel with the platform — no
-- Go-side map keyed by an installation-specific identifier. Defaults
-- to '{}' (no rules), which the validator treats as "skip"; seed rows
-- get filled in below.
ALTER TABLE platforms ADD COLUMN image_constraints TEXT NOT NULL DEFAULT '{}';

-- LinkedIn — JPEG/PNG/GIF, no animated GIFs, up to 9 images per post.
UPDATE platforms SET image_constraints = json_object(
    'max_file_size_bytes',      10485760,
    'allowed_formats',          json_array('jpeg','png','gif'),
    'animated_gif_supported',   json('false'),
    'max_attachments_per_post', 9
) WHERE id = 'AXqWG7U2qnpt';

-- YouTube — community posts only support a single image; treat other
-- post types the same for the MVP.
UPDATE platforms SET image_constraints = json_object(
    'max_file_size_bytes',      16777216,
    'allowed_formats',          json_array('jpeg','png','gif'),
    'animated_gif_supported',   json('true'),
    'max_attachments_per_post', 1
) WHERE id = '8S8bWQTG6qD';

-- Facebook — JPEG/PNG/GIF/WebP, animated GIFs allowed, up to 10.
UPDATE platforms SET image_constraints = json_object(
    'max_file_size_bytes',      31457280,
    'allowed_formats',          json_array('jpeg','png','gif','webp'),
    'animated_gif_supported',   json('true'),
    'max_attachments_per_post', 10
) WHERE id = 'zBU1zqVICGfk';

-- X (Twitter) — JPEG/PNG/WebP/GIF, animated GIFs allowed, up to 4.
UPDATE platforms SET image_constraints = json_object(
    'max_file_size_bytes',      5242880,
    'allowed_formats',          json_array('jpeg','png','webp','gif'),
    'animated_gif_supported',   json('true'),
    'max_attachments_per_post', 4
) WHERE id = '81mUCmc2xsKd';

-- Threads — JPEG/PNG/GIF, animated GIFs allowed, up to 20.
UPDATE platforms SET image_constraints = json_object(
    'max_file_size_bytes',      8388608,
    'allowed_formats',          json_array('jpeg','png','gif'),
    'animated_gif_supported',   json('true'),
    'max_attachments_per_post', 20
) WHERE id = 'pQ4yxT3SuE57';

-- Instagram — JPEG/PNG only, no GIFs, up to 20.
UPDATE platforms SET image_constraints = json_object(
    'max_file_size_bytes',      8388608,
    'allowed_formats',          json_array('jpeg','png'),
    'animated_gif_supported',   json('false'),
    'max_attachments_per_post', 20
) WHERE id = 'rzgpTkARLH0L';
