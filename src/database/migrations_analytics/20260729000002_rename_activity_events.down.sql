-- Reverse the tenant_activity_events rename.
ALTER INDEX idx_tenant_activity_events_tenant_entity_time   RENAME TO idx_activity_events_tenant_entity_time;
ALTER INDEX idx_tenant_activity_events_tenant_category_time RENAME TO idx_activity_events_tenant_category_time;
ALTER INDEX idx_tenant_activity_events_tenant_time          RENAME TO idx_activity_events_tenant_time;

ALTER TABLE tenant_activity_events RENAME TO activity_events;
