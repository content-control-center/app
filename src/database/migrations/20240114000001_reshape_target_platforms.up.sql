ALTER TABLE campaigns ADD COLUMN target_platforms TEXT NOT NULL DEFAULT '[]';

-- Convert ["id1","id2"] → [{"id":"id1","post_types":[]},{"id":"id2","post_types":[]}]
UPDATE campaigns SET target_platforms = (
    SELECT COALESCE(
        json_group_array(json_object('id', j.value, 'post_types', json('[]'))),
        '[]'
    )
    FROM json_each(campaigns.target_platform_ids) j
);

ALTER TABLE campaigns DROP COLUMN target_platform_ids;
