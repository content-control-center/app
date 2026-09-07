-- Reverse CON-217 headless connect-session state.
DROP INDEX IF EXISTS idx_zernio_connect_sessions_expires_at;
DROP TABLE IF EXISTS zernio_connect_sessions;
