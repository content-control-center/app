-- social_accounts holds the local view of accounts connected via the
-- Zernio integration (CON-62). The id matches Zernio's accountId so
-- inserts/updates are keyed off it directly. raw_json preserves the
-- full Zernio payload for forward compatibility.
--
-- Soft-delete (deleted_at IS NOT NULL) preserves FK integrity with
-- future Pieces/Posts that may reference an account that has since
-- been disconnected on Zernio's side.

CREATE TABLE social_accounts (
    id              TEXT      NOT NULL PRIMARY KEY,
    platform        TEXT      NOT NULL,
    profile_id      TEXT      NOT NULL,
    username        TEXT      NOT NULL DEFAULT '',
    display_name    TEXT      NOT NULL DEFAULT '',
    avatar_url      TEXT      NOT NULL DEFAULT '',
    is_active       BOOLEAN   NOT NULL DEFAULT 1,
    raw_json        TEXT      NOT NULL DEFAULT '{}',
    connected_at    TIMESTAMP NOT NULL,
    last_synced_at  TIMESTAMP NOT NULL,
    deleted_at      TIMESTAMP
);

-- Filtered indexes match the worker's hot-path queries (list active
-- accounts for a profile, list active accounts for a platform). NULL
-- comparison sees through SQLite's partial-index optimiser.
CREATE INDEX idx_social_accounts_platform_active
    ON social_accounts (platform)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_social_accounts_profile_active
    ON social_accounts (profile_id)
    WHERE deleted_at IS NULL;
