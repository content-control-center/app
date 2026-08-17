ALTER TABLE social_accounts
    DROP COLUMN token_expires_at,
    DROP COLUMN token_valid,
    DROP COLUMN health_status,
    DROP COLUMN needs_reconnect,
    DROP COLUMN last_health_checked_at;
