ALTER TABLE campaigns ADD COLUMN target_platform_ids TEXT NOT NULL DEFAULT '[]';

-- Convert [{"id":"id1","post_types":[...]}] back to ["id1"]
UPDATE campaigns SET target_platform_ids = (
    SELECT COALESCE(
        json_group_array(json_extract(j.value, '$.id')),
        '[]'
    )
    FROM json_each(campaigns.target_platforms) j
);

ALTER TABLE campaigns DROP COLUMN target_platforms;
