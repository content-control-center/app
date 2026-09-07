-- Rename activity_events → tenant_activity_events, mirroring the
-- vendor_usage_events rename, so the table name states its scope: the
-- tenant/user behavioural stream (CON-125).
--
-- As with vendor_usage_events, on TimescaleDB the RENAME carries the
-- hypertable, chunks, and compression/retention policies along; indexes are
-- renamed explicitly for consistency. All statements run identically on vanilla
-- Postgres (tests / dev), so no extension guard is required.
ALTER TABLE activity_events RENAME TO tenant_activity_events;

ALTER INDEX idx_activity_events_tenant_time          RENAME TO idx_tenant_activity_events_tenant_time;
ALTER INDEX idx_activity_events_tenant_category_time RENAME TO idx_tenant_activity_events_tenant_category_time;
ALTER INDEX idx_activity_events_tenant_entity_time   RENAME TO idx_tenant_activity_events_tenant_entity_time;
