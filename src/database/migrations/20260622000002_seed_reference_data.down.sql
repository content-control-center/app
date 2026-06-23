-- CON-87: remove the seeded reference data. Scoped to the exact rows the up
-- migration inserted, so rolling this back never touches platforms /
-- campaign types / phases added after the seed.
DELETE FROM campaigns_types_phases WHERE id IN ('M2', 'pr', 'so', '98', 'xh', '48', 'qp', 'Jg', 'PQ', 'C9', 'YH', 'Kj', 'hD', 'IO');
DELETE FROM campaigns_types WHERE id IN ('Uk', 'Ef', 'gb', 'uw', 'Vq');
DELETE FROM platforms WHERE id IN ('zBU1zqVICGfk', 'rzgpTkARLH0L', 'AXqWG7U2qnpt', 'pQ4yxT3SuE57', '81mUCmc2xsKd', '8S8bWQTG6qD');
DELETE FROM settings WHERE key = 'setup_complete';
