-- CON-182: campaign post-rate goal. estimated_post_count is reinterpreted as
-- the target number of posts PER period; goal_cadence sets the period the count
-- repeats over. Existing rows backfill to 'month' — their estimated_post_count
-- now multiplies by the number of months the campaign spans on the next
-- content-plan run (previously it was an absolute whole-campaign total).
ALTER TABLE campaigns
    ADD COLUMN goal_cadence TEXT NOT NULL DEFAULT 'month';
