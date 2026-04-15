ALTER TABLE assets_embeddings RENAME COLUMN asset_id TO piece_id;
ALTER TABLE campaigns RENAME COLUMN asset_ids TO pieces_ids;
ALTER TABLE campaigns RENAME COLUMN use_assets TO use_pieces;
ALTER TABLE posts RENAME COLUMN used_asset_ids TO used_pieces_ids;
