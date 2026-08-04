-- CON-123: correct seeded image_constraints to match Zernio's documented
-- per-platform limits.
--
-- The values seeded in 20260622000002_seed_reference_data disagree with what
-- the platforms actually accept; the front end has been compensating with a
-- hardcoded dictionary (ui/src/lib/platformMedia.ts). Verified 2026-08-03
-- against docs.zernio.com/llms-full.txt (see CON-123 verification comment).
--
-- Sizes are bytes: 8 MB = 8388608, 4 MB = 4194304, 1 MB = 1048576.
--
-- Surgical jsonb_set per changed key: nothing else writes image_constraints,
-- but touching only the wrong keys keeps the diff (and the down migration)
-- honest and leaves formats / gif flags untouched.
--
-- NOT covered here (needs follow-up):
--   * Step 1 of CON-123 — verifying against the raw Meta / LinkedIn / X APIs.
--     These are Zernio's documented limits; some (X's 1 MB strict cap,
--     Instagram/Threads auto-compression) look Zernio-imposed, not platform-
--     imposed.
--   * The X GIF-slot rule (1 GIF consumes all 4 image slots) — needs a shape
--     the constraint object does not have yet.

-- Instagram: carousel max 20 -> 10 images (1 feed post).
UPDATE platforms
    SET image_constraints = jsonb_set(image_constraints, '{max_attachments_per_post}', '10')
    WHERE id = 'rzgpTkARLH0L';

-- LinkedIn: 10 MB -> 8 MB, max 9 -> 20 images.
UPDATE platforms
    SET image_constraints = jsonb_set(
        jsonb_set(image_constraints, '{max_file_size_bytes}', '8388608'),
        '{max_attachments_per_post}', '20')
    WHERE id = 'AXqWG7U2qnpt';

-- Facebook: 30 MB -> 4 MB (larger images rejected in practice).
UPDATE platforms
    SET image_constraints = jsonb_set(image_constraints, '{max_file_size_bytes}', '4194304')
    WHERE id = 'zBU1zqVICGfk';

-- Threads: carousel max 20 -> 10 images.
UPDATE platforms
    SET image_constraints = jsonb_set(image_constraints, '{max_attachments_per_post}', '10')
    WHERE id = 'pQ4yxT3SuE57';

-- X (Twitter): 5 MB -> 1 MB per image (strict).
UPDATE platforms
    SET image_constraints = jsonb_set(image_constraints, '{max_file_size_bytes}', '1048576')
    WHERE id = '81mUCmc2xsKd';
