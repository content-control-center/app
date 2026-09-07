-- CON-182: drop the campaign goal cadence.
ALTER TABLE campaigns
    DROP COLUMN IF EXISTS goal_cadence;
