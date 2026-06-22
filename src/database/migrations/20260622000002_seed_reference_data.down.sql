-- CON-87: remove the seeded reference data.
DELETE FROM campaigns_types_phases;
DELETE FROM campaigns_types;
DELETE FROM platforms;
DELETE FROM settings WHERE key = 'setup_complete';
