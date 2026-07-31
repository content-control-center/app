-- CON-91: encode per-platform / per-post-type text (character) limits.
--
-- text_constraints mirrors image_constraints / pdf_constraints / video_
-- constraints: a jsonb rule set on the platform row. A zero value ('{}')
-- means "this platform has no text limit". max_content_chars is the default
-- body ceiling; per_post_type overrides it per slug (e.g. LinkedIn articles);
-- max_title_chars caps a distinct title field (YouTube). Counts are Unicode
-- code points, not bytes.
--
-- Values are derived from the human-readable `constraints` prose already on
-- each row; CON-123 verifies them against the real platform limits. Seeded by
-- id so the backfill is stable regardless of name changes.

ALTER TABLE platforms
    ADD COLUMN text_constraints jsonb NOT NULL DEFAULT '{}';

-- Facebook: posts up to ~63k chars.
UPDATE platforms SET text_constraints =
    '{"max_content_chars":63000}'
    WHERE id = 'zBU1zqVICGfk';

-- Instagram: captions up to 2200 chars.
UPDATE platforms SET text_constraints =
    '{"max_content_chars":2200}'
    WHERE id = 'rzgpTkARLH0L';

-- LinkedIn: feed posts up to 3000 chars; native articles up to ~100k.
UPDATE platforms SET text_constraints =
    '{"max_content_chars":3000,"per_post_type":{"article":100000}}'
    WHERE id = 'AXqWG7U2qnpt';

-- Threads: posts up to 500 chars.
UPDATE platforms SET text_constraints =
    '{"max_content_chars":500}'
    WHERE id = 'pQ4yxT3SuE57';

-- X (Twitter): posts up to 280 chars; long-form (Premium) up to 25k.
UPDATE platforms SET text_constraints =
    '{"max_content_chars":280,"per_post_type":{"long-form-post":25000}}'
    WHERE id = '81mUCmc2xsKd';

-- YouTube: descriptions up to 5000 chars; titles up to 100 chars.
UPDATE platforms SET text_constraints =
    '{"max_content_chars":5000,"max_title_chars":100}'
    WHERE id = '8S8bWQTG6qD';
