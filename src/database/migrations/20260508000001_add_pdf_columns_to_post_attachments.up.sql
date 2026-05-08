-- CON-75: post_attachments now also stores PDFs (e.g. LinkedIn carousel
-- documents). page_count is meaningful only for PDFs (image rows leave
-- it at the default 0). thumbnail_s3_key points at the rendered first
-- page PNG for PDF rows; image rows leave it NULL.
ALTER TABLE post_attachments ADD COLUMN page_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE post_attachments ADD COLUMN thumbnail_s3_key TEXT;
