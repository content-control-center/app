ALTER TABLE platforms ADD COLUMN cadence     TEXT NOT NULL DEFAULT '';
ALTER TABLE platforms ADD COLUMN constraints TEXT NOT NULL DEFAULT '';

UPDATE platforms SET
    cadence     = '1–2 posts per week',
    constraints = 'professional tone; posts up to 3000 chars; articles up to 100k chars'
WHERE id = 'linkedin';

UPDATE platforms SET
    cadence     = '3–5 posts per week',
    constraints = 'posts max 280 chars; use threads for longer content'
WHERE id = 'x-twitter';

UPDATE platforms SET
    cadence     = '3–5 posts per week',
    constraints = 'varied formats; posts up to 63k chars; balanced tone'
WHERE id = 'facebook';

UPDATE platforms SET
    cadence     = '4–7 posts per week',
    constraints = 'visual-first; captions up to 2200 chars; strong opening line'
WHERE id = 'instagram';

UPDATE platforms SET
    cadence     = '3–5 posts per week',
    constraints = 'posts up to 500 chars; conversational tone'
WHERE id = 'threads';

UPDATE platforms SET
    cadence     = '1 video per week',
    constraints = 'video-first; titles up to 100 chars; descriptions up to 5000 chars'
WHERE id = 'youtube';
