-- post_analytics holds the latest engagement snapshot for a post Ogen
-- published through a publisher (CON-93). Exactly one row per Post —
-- post_id PRIMARY KEY — so the background refresh queue UPSERTs the
-- newest numbers rather than versioning; analytics history is an
-- explicitly deferred concern (§13). FK CASCADE drops the snapshot when
-- its Post is deleted.
--
-- The aggregate metrics (impressions … views, engagement_rate) are
-- denormalised into their own columns because the list/overview view
-- sorts and sums them without parsing JSON. The per-platform breakdown
-- — each platform's status, ids, urls, per-platform sync_status /
-- error_message (the 412/reauthorizeUrl scope-gap nuance), and its own
-- metrics — is kept together in the platform_analytics JSON column; a
-- normalised per-platform table is deferred to a future iteration.
--
-- publisher_post_id is denormalised from posts so the refresh queue can
-- match Zernio's returned items back to a local post by id; UNIQUE keeps
-- it 1:1 with the owning post. metrics_last_updated is Zernio's
-- analytics.lastUpdated (when Zernio last computed the numbers);
-- last_refreshed_at is when Ogen last fetched them — together they make
-- staleness visible. raw_json retains the verbatim Zernio item for
-- forward-compat.
CREATE TABLE post_analytics
(
    post_id              TEXT     PRIMARY KEY REFERENCES posts (id) ON DELETE CASCADE,
    publisher_post_id    TEXT     NOT NULL UNIQUE,
    impressions          INTEGER  NOT NULL DEFAULT 0,
    reach                INTEGER  NOT NULL DEFAULT 0,
    likes                INTEGER  NOT NULL DEFAULT 0,
    comments             INTEGER  NOT NULL DEFAULT 0,
    shares               INTEGER  NOT NULL DEFAULT 0,
    saves                INTEGER  NOT NULL DEFAULT 0,
    clicks               INTEGER  NOT NULL DEFAULT 0,
    views                INTEGER  NOT NULL DEFAULT 0,
    engagement_rate      REAL     NOT NULL DEFAULT 0,
    platform_analytics   TEXT     NOT NULL DEFAULT '[]',
    sync_status          TEXT     NOT NULL DEFAULT '',
    metrics_last_updated DATETIME,
    last_refreshed_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    raw_json             TEXT     NOT NULL DEFAULT '{}',
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
