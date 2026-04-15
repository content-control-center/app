ALTER TABLE assets_embeddings RENAME COLUMN piece_id TO asset_id;
ALTER TABLE campaigns RENAME COLUMN pieces_ids TO asset_ids;
ALTER TABLE campaigns RENAME COLUMN use_pieces TO use_assets;
ALTER TABLE posts RENAME COLUMN used_pieces_ids TO used_asset_ids;
