ALTER TABLE assets ADD COLUMN status TEXT NOT NULL DEFAULT 'ready'
    CHECK (status IN ('pending', 'processing', 'ready', 'failed'));

ALTER TABLE assets ADD COLUMN type TEXT
    CHECK (type IS NULL OR type IN ('MD'));
