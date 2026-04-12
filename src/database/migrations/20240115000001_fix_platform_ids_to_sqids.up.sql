-- Migration 20240113 failed (FK constraint) and was rolled back, leaving platform
-- IDs as old slugs. Migration 20240114 then ran and converted target_platform_ids
-- → target_platforms using those old slugs. This migration completes the rename.
--
-- Uses expand-and-contract to avoid FK violations:
--   insert new Sqid rows → migrate child refs → delete old rows.

-- Step 1: insert new rows with Sqid IDs, copying all columns.
INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT 'AXqWG7U2qnpt', name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = 'linkedin';
INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT '8S8bWQTG6qD',  name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = 'youtube';
INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT 'zBU1zqVICGfk', name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = 'facebook';
INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT '81mUCmc2xsKd', name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = 'x-twitter';
INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT 'pQ4yxT3SuE57', name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = 'threads';
INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT 'rzgpTkARLH0L', name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = 'instagram';

-- Step 2: move posts.platform_id to the new IDs.
UPDATE posts SET platform_id = 'AXqWG7U2qnpt' WHERE platform_id = 'linkedin';
UPDATE posts SET platform_id = '8S8bWQTG6qD'  WHERE platform_id = 'youtube';
UPDATE posts SET platform_id = 'zBU1zqVICGfk' WHERE platform_id = 'facebook';
UPDATE posts SET platform_id = '81mUCmc2xsKd' WHERE platform_id = 'x-twitter';
UPDATE posts SET platform_id = 'pQ4yxT3SuE57' WHERE platform_id = 'threads';
UPDATE posts SET platform_id = 'rzgpTkARLH0L' WHERE platform_id = 'instagram';

-- Step 3: rewrite the "id" field inside the campaigns.target_platforms JSON array.
UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"linkedin"',  '"id":"AXqWG7U2qnpt"');
UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"youtube"',   '"id":"8S8bWQTG6qD"');
UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"facebook"',  '"id":"zBU1zqVICGfk"');
UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"x-twitter"', '"id":"81mUCmc2xsKd"');
UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"threads"',   '"id":"pQ4yxT3SuE57"');
UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"instagram"', '"id":"rzgpTkARLH0L"');

-- Step 4: drop old rows (no child rows reference them any more).
DELETE FROM platforms WHERE id IN ('linkedin', 'youtube', 'facebook', 'x-twitter', 'threads', 'instagram');
