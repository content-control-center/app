-- Dropping the table removes its hypertable, compression, and retention
-- policies with it (TimescaleDB cascades those to the parent table).
DROP TABLE IF EXISTS follower_stats_snapshots;
