-- CON-219: Zernio account-health snapshot. A periodic sweep reads each
-- connected account's Zernio health (GET /v1/accounts/health) and persists the
-- forward-looking token expiry + status here, so the app can warn workspace
-- owners before a connection lapses (and, later, surface health in the UI).
-- All columns are nullable and backfill-free: a row carries NULLs until the
-- first health sweep populates it. The reconciler's upsert (ApplyPlan) touches
-- only the mirror columns, so a sync never clobbers a health snapshot.
ALTER TABLE social_accounts
    ADD COLUMN token_expires_at       TIMESTAMPTZ,
    ADD COLUMN token_valid            BOOLEAN,
    ADD COLUMN health_status          TEXT,
    ADD COLUMN needs_reconnect        BOOLEAN,
    ADD COLUMN last_health_checked_at TIMESTAMPTZ;
