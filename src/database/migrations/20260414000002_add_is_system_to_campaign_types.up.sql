ALTER TABLE campaign_types ADD COLUMN is_system BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE campaign_types SET is_system = TRUE WHERE id IN ('Uk', 'gb', 'Ef', 'Vq');
