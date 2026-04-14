-- Rebuild campaigns: restore objective, drop campaign_type_id
CREATE TABLE campaigns_old
(
    id                   TEXT     PRIMARY KEY,
    name                 TEXT     NOT NULL,
    description          TEXT     NOT NULL DEFAULT '',
    target_persona       TEXT     NOT NULL DEFAULT '',
    key_messages         TEXT     NOT NULL DEFAULT '',
    tone_guidelines      TEXT     NOT NULL DEFAULT '',
    use_pieces           BOOLEAN  NOT NULL DEFAULT FALSE,
    pieces_ids           TEXT     NOT NULL DEFAULT '[]',
    target_platforms     TEXT     NOT NULL DEFAULT '[]',
    objective            TEXT     NOT NULL,
    status               TEXT     NOT NULL DEFAULT 'draft',
    start_date           DATETIME,
    end_date             DATETIME,
    estimated_post_count INTEGER,
    budget               REAL,
    currency             TEXT     NOT NULL DEFAULT '',
    language             TEXT     NOT NULL DEFAULT '',
    tag_ids              TEXT     NOT NULL DEFAULT '[]',
    created_by           TEXT     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO campaigns_old
SELECT c.id,
       c.name,
       c.description,
       c.target_persona,
       c.key_messages,
       c.tone_guidelines,
       c.use_pieces,
       c.pieces_ids,
       c.target_platforms,
       COALESCE((SELECT ct.name FROM campaign_types ct WHERE ct.id = c.campaign_type_id), ''),
       c.status,
       c.start_date,
       c.end_date,
       c.estimated_post_count,
       c.budget,
       c.currency,
       c.language,
       c.tag_ids,
       c.created_by,
       c.created_at,
       c.updated_at
FROM campaigns c;

DROP TABLE campaigns;
ALTER TABLE campaigns_old RENAME TO campaigns;

DROP TABLE campaign_type_phases;
DROP TABLE campaign_types;
