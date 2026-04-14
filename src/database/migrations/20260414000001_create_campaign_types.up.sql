CREATE TABLE campaign_types
(
    id          TEXT     PRIMARY KEY,
    name        TEXT     NOT NULL UNIQUE,
    label       TEXT     NOT NULL DEFAULT '',
    description TEXT     NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE campaign_type_phases
(
    id               TEXT     PRIMARY KEY,
    campaign_type_id TEXT     NOT NULL REFERENCES campaign_types (id) ON DELETE CASCADE,
    name             TEXT     NOT NULL,
    purpose          TEXT     NOT NULL DEFAULT '',
    sequence         INTEGER  NOT NULL,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ── Seed campaign types ───────────────────────────────────────────────────────

INSERT INTO campaign_types (id, name, label, description) VALUES
    ('ct_awareness',  'awareness',  'Awareness',  'Increase brand/product visibility among new audiences.'),
    ('ct_engagement', 'engagement', 'Engagement', 'Build interactive relationships with the audience; increase frequency and depth of interaction.'),
    ('ct_conversion', 'conversion', 'Conversion', 'Drive a specific action — purchase, signup, demo request, subscription.'),
    ('ct_retention',  'retention',  'Retention',  'Keep existing customers active, loyal, and expanding usage.');

-- ── Seed phases ───────────────────────────────────────────────────────────────

-- Awareness (2 phases)
INSERT INTO campaign_type_phases (id, campaign_type_id, name, purpose, sequence) VALUES
    ('ctp_aw_1', 'ct_awareness', 'Launch & Distribution',
     'Generate initial reach through high-impact content. Hero content (anchor piece carrying the campaign narrative), hub content (supporting pieces that elaborate on the hero''s theme — blog posts, infographics, expert interviews), hygiene/help content (searchable evergreen pieces — how-tos, explainers, glossaries), social-native content (platform-specific short-form — reels, carousels, stories, threads), paid creative (display ads, sponsored posts, pre-roll).',
     1),
    ('ctp_aw_2', 'ct_awareness', 'Sustain & Optimize',
     'Maintain momentum and extend campaign reach. Refreshed variants (new angles/formats of existing assets to combat fatigue), user-generated and earned content (amplifying mentions, resharing audience reactions), retargeting content (sequenced messaging for people who engaged earlier), recap and proof content (milestone posts, highlight reels, social proof).',
     2);

-- Engagement (3 phases)
INSERT INTO campaign_type_phases (id, campaign_type_id, name, purpose, sequence) VALUES
    ('ctp_en_1', 'ct_engagement', 'Activate',
     'Spark the first interaction. Conversation starters (polls, questions, opinion prompts), interactive content (quizzes, calculators, assessments), lead magnets (templates, checklists, mini-courses), live event content (webinar scripts, AMA prompts, live stream outlines), community seeding content (founding posts, tone-setting discussions).',
     1),
    ('ctp_en_2', 'ct_engagement', 'Deepen',
     'Increase engagement frequency and emotional investment. Serialized content (multi-part series, episodic content, recurring formats), UGC campaign content (challenges, contests, hashtag campaigns), collaborative content (co-created posts, expert roundups, guest takeovers, spotlights), exclusive access content (early previews, insider updates, members-only drops), personalized sequences (email nurture flows, segmented content paths).',
     2),
    ('ctp_en_3', 'ct_engagement', 'Sustain & Compound',
     'Make engagement self-reinforcing. Community-driven content (curated discussions, moderation prompts), feedback loop content (surveys, advisory board invitations), recognition content (leaderboards, contributor spotlights, badges), re-engagement content (win-back emails, fresh entry points for lapsed participants), bridge content (pieces introducing conversion themes — case studies, "how others use it" stories).',
     3);

-- Conversion (3 phases)
INSERT INTO campaign_type_phases (id, campaign_type_id, name, purpose, sequence) VALUES
    ('ctp_co_1', 'ct_conversion', 'Capture',
     'Catch intent and pull prospects into a decision-making context. Landing page copy (single-CTA, segment-specific), lead capture content (whitepapers, ROI calculators, free trial prompts, demo CTAs), retargeting ad copy (sequenced benefit-driven creative), search-intent content (comparison pages, "best X for Y" guides, alternative-to pages, pricing breakdowns), social proof assets (testimonials, case study summaries, review compilations, trust badge copy).',
     1),
    ('ctp_co_2', 'ct_conversion', 'Nurture & Persuade',
     'Build the case for prospects who didn''t convert on first contact. Email nurture sequences (value → social proof → objection handling → direct offer), full case studies and success stories (industry/role-matched), objection-handling content (FAQ copy, myth-busting posts, security/compliance docs, comparison charts), demo and walkthrough content (product tour scripts, video walkthrough scripts, sandbox guides), urgency and scarcity content (limited-time offer copy, countdown messaging), personalized outreach templates (abandoned-cart emails, pricing-page-visit triggers).',
     2),
    ('ctp_co_3', 'ct_conversion', 'Close & Validate',
     'Final push and immediate post-conversion reassurance. Last-mile content (guarantees, "cancel anytime" messaging, security certifications, real-time social proof copy), onboarding content (welcome emails, getting-started guides, first-value tutorials, setup wizard copy), confirmation and celebration content (order confirmations, thank-you page copy, "you''re in" emails), feedback capture content (post-purchase survey copy, "how did you hear about us" prompts).',
     3);

-- Retention (3 phases)
INSERT INTO campaign_type_phases (id, campaign_type_id, name, purpose, sequence) VALUES
    ('ctp_re_1', 'ct_retention', 'Activate & Embed',
     'Establish habits and deliver first value post-conversion. Onboarding sequences (step-by-step guides, welcome email series, first-use tutorials), quick-win content (short tutorials, "do this in 5 minutes" templates, starter configurations), milestone celebration content ("you''ve completed your first X," progress updates), integration and ecosystem content (tool connection guides, workflow templates, API quickstarts), assigned support content (account manager introductions, personalized check-in emails).',
     1),
    ('ctp_re_2', 'ct_retention', 'Deepen & Expand',
     'Increase usage breadth and emotional investment. Advanced feature education ("did you know..." emails, feature-specific tutorials), use case content (stories showing different ways to use the product beyond initial purpose), community content (forum prompts, user group announcements, meetup/event content), exclusive and loyalty content (early access announcements, insider roadmap previews, loyalty rewards, anniversary perks), customer education content (certification materials, masterclass outlines, learning path descriptions), co-creation content (beta program invitations, feature request prompts, advisory council content).',
     2),
    ('ctp_re_3', 'ct_retention', 'Sustain & Recover',
     'Maintain relationship and prevent/reverse churn. Health-monitoring content (usage report copy, ROI summary templates, "your year in review" copy), proactive check-in content ("how''s it going" emails, QBR preparation docs, satisfaction survey copy), re-engagement sequences (usage-drop triggers, "here''s what''s new" updates, help offers), win-back campaigns (exit survey copy, special offers, "here''s what''s changed" updates, personal outreach templates), advocacy and referral content (referral program copy, review requests, case study invitations, ambassador program materials), renewal and upgrade content (renewal reminders, upgrade path comparisons, ROI justification templates).',
     3);

-- ── Rebuild campaigns: replace objective with campaign_type_id ────────────────

CREATE TABLE campaigns_new
(
    id                   TEXT     PRIMARY KEY,
    name                 TEXT     NOT NULL,
    description          TEXT     NOT NULL DEFAULT '',
    target_persona       TEXT     NOT NULL DEFAULT '',
    key_messages         TEXT     NOT NULL DEFAULT '',
    tone_guidelines      TEXT     NOT NULL DEFAULT '',
    use_pieces           BOOLEAN  NOT NULL DEFAULT FALSE,
    pieces_ids           TEXT     NOT NULL DEFAULT '[]',
    target_platforms     TEXT     NOT NULL DEFAULT '[]',
    campaign_type_id     TEXT     NOT NULL REFERENCES campaign_types (id),
    status               TEXT     NOT NULL DEFAULT 'draft',
    start_date           DATETIME,
    end_date             DATETIME,
    estimated_post_count INTEGER,
    budget               REAL,
    currency             TEXT     NOT NULL DEFAULT '',
    language             TEXT     NOT NULL DEFAULT '',
    tag_ids              TEXT     NOT NULL DEFAULT '[]',
    created_by           TEXT     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO campaigns_new
SELECT c.id,
       c.name,
       c.description,
       c.target_persona,
       c.key_messages,
       c.tone_guidelines,
       c.use_pieces,
       c.pieces_ids,
       c.target_platforms,
       COALESCE((SELECT ct.id FROM campaign_types ct WHERE ct.name = c.objective), 'ct_awareness'),
       c.status,
       c.start_date,
       c.end_date,
       c.estimated_post_count,
       c.budget,
       c.currency,
       c.language,
       c.tag_ids,
       c.created_by,
       c.created_at,
       c.updated_at
FROM campaigns c;

DROP TABLE campaigns;
ALTER TABLE campaigns_new RENAME TO campaigns;
