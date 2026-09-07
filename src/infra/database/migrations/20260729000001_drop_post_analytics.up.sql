-- CON-125 Track B — final step: drop the obsolete main-DB post_analytics table.
--
-- Post engagement snapshots now live append-only in the ISOLATED analytics
-- database (post_analytics_snapshots); models.PostAnalytics maps there and the
-- app no longer reads or writes this table. The boot-time BackfillPostAnalytics
-- copied every historical row across before this migration (that backfill and
-- its wiring are removed in the same change), so the drop loses no data.
DROP TABLE IF EXISTS post_analytics;
