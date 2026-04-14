INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT 'linkedin',  name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = 'AXqWG7U2qnpt';
INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT 'youtube',   name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = '8S8bWQTG6qD';
INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT 'facebook',  name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = 'zBU1zqVICGfk';
INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT 'x-twitter', name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = '81mUCmc2xsKd';
INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT 'threads',   name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = 'pQ4yxT3SuE57';
INSERT INTO platforms (id, name, post_types, cadence, constraints, created_at, updated_at)
    SELECT 'instagram', name, post_types, cadence, constraints, created_at, updated_at FROM platforms WHERE id = 'rzgpTkARLH0L';

UPDATE posts SET platform_id = 'linkedin'  WHERE platform_id = 'AXqWG7U2qnpt';
UPDATE posts SET platform_id = 'youtube'   WHERE platform_id = '8S8bWQTG6qD';
UPDATE posts SET platform_id = 'facebook'  WHERE platform_id = 'zBU1zqVICGfk';
UPDATE posts SET platform_id = 'x-twitter' WHERE platform_id = '81mUCmc2xsKd';
UPDATE posts SET platform_id = 'threads'   WHERE platform_id = 'pQ4yxT3SuE57';
UPDATE posts SET platform_id = 'instagram' WHERE platform_id = 'rzgpTkARLH0L';

UPDATE campaigns SET target_platform_ids = REPLACE(target_platform_ids, '"AXqWG7U2qnpt"', '"linkedin"');
UPDATE campaigns SET target_platform_ids = REPLACE(target_platform_ids, '"8S8bWQTG6qD"',  '"youtube"');
UPDATE campaigns SET target_platform_ids = REPLACE(target_platform_ids, '"zBU1zqVICGfk"', '"facebook"');
UPDATE campaigns SET target_platform_ids = REPLACE(target_platform_ids, '"81mUCmc2xsKd"', '"x-twitter"');
UPDATE campaigns SET target_platform_ids = REPLACE(target_platform_ids, '"pQ4yxT3SuE57"', '"threads"');
UPDATE campaigns SET target_platform_ids = REPLACE(target_platform_ids, '"rzgpTkARLH0L"', '"instagram"');

DELETE FROM platforms WHERE id IN ('AXqWG7U2qnpt', '8S8bWQTG6qD', 'zBU1zqVICGfk', '81mUCmc2xsKd', 'pQ4yxT3SuE57', 'rzgpTkARLH0L');
