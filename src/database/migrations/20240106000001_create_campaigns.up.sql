CREATE TABLE campaigns
(
    id                  TEXT     PRIMARY KEY,
    name                TEXT     NOT NULL,
    description         TEXT     NOT NULL DEFAULT '',
    target_persona      TEXT     NOT NULL DEFAULT '',
    key_messages        TEXT     NOT NULL DEFAULT '',
    tone_guidelines     TEXT     NOT NULL DEFAULT '',
    use_pieces          BOOLEAN  NOT NULL DEFAULT FALSE,
    pieces_ids          TEXT     NOT NULL DEFAULT '[]',
    target_platform_ids TEXT     NOT NULL DEFAULT '[]',
    objective           TEXT     NOT NULL,
    status              TEXT     NOT NULL DEFAULT 'draft',
    start_date          DATETIME,
    end_date            DATETIME,
    budget              REAL,
    currency            TEXT     NOT NULL DEFAULT '',
    language            TEXT     NOT NULL DEFAULT '',
    tag_ids             TEXT     NOT NULL DEFAULT '[]',
    created_by          TEXT     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
