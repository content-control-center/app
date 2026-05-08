-- Per-platform PDF attachment constraints (CON-75). Sibling to
-- image_constraints; lives on the platform row so the rules travel
-- with the platform. Defaults to '{}' (no PDF support), which the
-- validator surfaces as a soft warning when a PDF is attached for
-- a platform that does not support PDFs.
ALTER TABLE platforms ADD COLUMN pdf_constraints TEXT NOT NULL DEFAULT '{}';

-- LinkedIn — carousel/document posts. Up to ~100 MB, ~300 pages,
-- one PDF per post.
UPDATE platforms SET pdf_constraints = json_object(
    'max_file_size_bytes',      104857600,
    'allowed_formats',          json_array('pdf'),
    'max_pages',                300,
    'max_attachments_per_post', 1
) WHERE id = 'AXqWG7U2qnpt';
