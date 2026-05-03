-- auto_publish_allowlist holds the workspace-scoped set A of platforms
-- on which Ogen may auto-publish Scheduled posts (CON-65). Single-row
-- per platform; the empty table means "auto-publishing disabled
-- everywhere" (opt-in default).
--
-- platform_id stores Zernio's wire identifier (e.g. "linkedin",
-- "twitter") — the same string accepted by POST
-- /api/integrations/zernio/connect-links. It is intentionally not FK'd
-- to platforms(id): platforms.id is a Sqid (e.g. "AXqWG7U2qnpt"), a
-- different identifier system. Z-membership is enforced in the
-- repository layer against zernio.LookupSupportedPlatform.
CREATE TABLE auto_publish_allowlist (
    platform_id TEXT     NOT NULL PRIMARY KEY,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
