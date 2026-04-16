UPDATE posts SET status = 'in_review' WHERE status = 'ready_for_publish';
UPDATE posts SET status = 'draft' WHERE status = 'scheduled_for_manual_publishing';
UPDATE posts SET status = 'draft' WHERE status = 'not_published';
