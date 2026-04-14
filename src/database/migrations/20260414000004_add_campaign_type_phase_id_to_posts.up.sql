ALTER TABLE posts ADD COLUMN campaign_type_phase_id TEXT REFERENCES campaigns_types_phases (id);
