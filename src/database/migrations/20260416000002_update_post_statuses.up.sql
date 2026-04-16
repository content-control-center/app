UPDATE posts SET status = 'ready_for_publish' WHERE status = 'in_review';
UPDATE posts SET status = 'ready_for_publish' WHERE status = 'approved';
