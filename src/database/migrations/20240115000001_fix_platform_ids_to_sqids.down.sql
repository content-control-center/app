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

UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"AXqWG7U2qnpt"', '"id":"linkedin"');
UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"8S8bWQTG6qD"',  '"id":"youtube"');
UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"zBU1zqVICGfk"', '"id":"facebook"');
UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"81mUCmc2xsKd"', '"id":"x-twitter"');
UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"pQ4yxT3SuE57"', '"id":"threads"');
UPDATE campaigns SET target_platforms = REPLACE(target_platforms, '"id":"rzgpTkARLH0L"', '"id":"instagram"');

DELETE FROM platforms WHERE id IN ('AXqWG7U2qnpt', '8S8bWQTG6qD', 'zBU1zqVICGfk', '81mUCmc2xsKd', 'pQ4yxT3SuE57', 'rzgpTkARLH0L');
