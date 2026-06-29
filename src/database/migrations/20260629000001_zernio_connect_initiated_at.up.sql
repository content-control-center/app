-- CON-102: gate the Zernio sync sweep on connection-initiation, not profile
-- presence.
--
-- Eager provisioning (the bootstrap_zernio_profile job) gives every tenant a
-- zernio.profile_id at signup, so the sync worker can no longer enumerate
-- "tenants that use Zernio" by profile presence — that would now match every
-- tenant. It switches to the zernio.connect_initiated_at marker (written on a
-- tenant's first connect-link). Backfill that marker for every tenant that
-- already has a profile (pre-CON-102 a profile was only ever created by
-- connecting), so their sync coverage is preserved across the deploy. The value
-- mirrors the profile's recorded creation time when present, else now() in
-- RFC3339 UTC. settings is keyed (tenant_id, key); reference data lives under
-- tenant 'default'.
INSERT INTO settings (tenant_id, key, value)
SELECT
    p.tenant_id,
    'zernio.connect_initiated_at',
    COALESCE(
        NULLIF(c.value, ''),
        to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
    )
FROM settings p
LEFT JOIN settings c
    ON c.tenant_id = p.tenant_id
   AND c.key = 'zernio.profile_created_at'
WHERE p.key = 'zernio.profile_id'
  AND p.value <> ''
ON CONFLICT (tenant_id, key) DO NOTHING;
