-- Rename usage_events → vendor_usage_events to make the table's scope explicit:
-- it records metered VENDOR calls (models + publishers, CON-86), and now sits
-- beside the renamed tenant_activity_events behavioural stream.
--
-- On TimescaleDB the RENAME carries the hypertable, its chunks, and the
-- compression + retention policies along automatically (they are keyed by the
-- hypertable's relation, not its name). Indexes keep their old names after a
-- table rename, so they are renamed explicitly below for consistency. Both
-- statement kinds run identically on vanilla Postgres (tests / dev), so no
-- extension guard is needed here.
ALTER TABLE usage_events RENAME TO vendor_usage_events;

ALTER INDEX idx_usage_events_tenant_time        RENAME TO idx_vendor_usage_events_tenant_time;
ALTER INDEX idx_usage_events_tenant_vendor_time RENAME TO idx_vendor_usage_events_tenant_vendor_time;
