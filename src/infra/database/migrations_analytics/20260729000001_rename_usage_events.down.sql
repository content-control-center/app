-- Reverse the vendor_usage_events rename.
ALTER INDEX idx_vendor_usage_events_tenant_vendor_time RENAME TO idx_usage_events_tenant_vendor_time;
ALTER INDEX idx_vendor_usage_events_tenant_time        RENAME TO idx_usage_events_tenant_time;

ALTER TABLE vendor_usage_events RENAME TO usage_events;
