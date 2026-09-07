-- CON-122: per-attachment alt text (accessibility; also sent to Zernio as
-- mediaItems[].altText). NOT NULL DEFAULT '' so existing rows and uploads that
-- omit it read back as "no alt text" rather than NULL.
ALTER TABLE post_attachments ADD COLUMN alt_text TEXT NOT NULL DEFAULT '';
