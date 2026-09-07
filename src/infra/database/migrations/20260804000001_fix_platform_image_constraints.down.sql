-- Revert to the values seeded in 20260622000002_seed_reference_data.

-- Instagram: 10 -> 20 images.
UPDATE platforms
    SET image_constraints = jsonb_set(image_constraints, '{max_attachments_per_post}', '20')
    WHERE id = 'rzgpTkARLH0L';

-- LinkedIn: 8 MB -> 10 MB, 20 -> 9 images.
UPDATE platforms
    SET image_constraints = jsonb_set(
        jsonb_set(image_constraints, '{max_file_size_bytes}', '10485760'),
        '{max_attachments_per_post}', '9')
    WHERE id = 'AXqWG7U2qnpt';

-- Facebook: 4 MB -> 30 MB.
UPDATE platforms
    SET image_constraints = jsonb_set(image_constraints, '{max_file_size_bytes}', '31457280')
    WHERE id = 'zBU1zqVICGfk';

-- Threads: 10 -> 20 images.
UPDATE platforms
    SET image_constraints = jsonb_set(image_constraints, '{max_attachments_per_post}', '20')
    WHERE id = 'pQ4yxT3SuE57';

-- X (Twitter): 1 MB -> 5 MB.
UPDATE platforms
    SET image_constraints = jsonb_set(image_constraints, '{max_file_size_bytes}', '5242880')
    WHERE id = '81mUCmc2xsKd';
