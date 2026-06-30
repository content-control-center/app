-- CON-86: drop usage_events. On TimescaleDB this also removes the hypertable
-- chunks and the compression/retention policies attached to the table.
DROP TABLE IF EXISTS usage_events;
