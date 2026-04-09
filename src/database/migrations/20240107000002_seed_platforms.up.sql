INSERT INTO platforms (id, name, post_types) VALUES
(
    'linkedin',
    'LinkedIn',
    '{"text-post":"Text post","image-post":"Image post","carousel":"Carousel (PDF document)","video":"Video","article":"Article (long-form native blog)","poll":"Poll","newsletter":"Newsletter","event":"Event","live-video":"Live video"}'
),
(
    'youtube',
    'YouTube',
    '{"video":"Video (standard long-form)","short":"Short (≤60s vertical)","live-stream":"Live stream","premiere":"Premiere (scheduled video with live chat)","community-post":"Community post (text/image/poll in the Community tab)","podcast":"Podcast (audio-first format via YouTube Music integration)"}'
),
(
    'facebook',
    'Facebook',
    '{"text-post":"Text post","image-post":"Image post","video":"Video","reel":"Reel (≤90s vertical)","story":"Story (24h ephemeral)","live-video":"Live video","carousel":"Carousel (multi-image)","poll":"Poll","event":"Event","link-post":"Link post (with OG preview card)"}'
),
(
    'x-twitter',
    'X (Twitter)',
    '{"text-post":"Text post","image-post":"Image post","video":"Video","long-form-post":"Long-form post (formerly Twitter Notes / Articles)","poll":"Poll","space":"Space (live audio)","thread":"Thread (multi-post sequence)"}'
),
(
    'threads',
    'Threads',
    '{"text-post":"Text post","image-post":"Image post","carousel":"Carousel (up to 20 images/videos)","video":"Video","poll":"Poll","gif-post":"GIF post"}'
),
(
    'instagram',
    'Instagram',
    '{"image-post":"Image post","carousel":"Carousel (up to 20 slides)","reel":"Reel (up to 15 min)","story":"Story (24h ephemeral)","live-video":"Live video","broadcast-channel":"Broadcast Channel (one-to-many messaging)","collaborative-post":"Collaborative post (co-authored)","guide":"Guide (curated collection)"}'
);
