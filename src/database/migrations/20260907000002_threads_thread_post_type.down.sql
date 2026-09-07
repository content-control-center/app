-- CON-284 rollback: remove the 'thread' post type from Meta Threads.
UPDATE platforms
SET post_types = post_types - 'thread'
WHERE id = 'pQ4yxT3SuE57';
