-- CON-102 down: remove the connect-initiation markers. Rolled back together with
-- the application, the sync worker reverts to enumerating tenants by
-- zernio.profile_id, so these per-tenant markers are no longer consulted.
DELETE FROM settings WHERE key = 'zernio.connect_initiated_at';
