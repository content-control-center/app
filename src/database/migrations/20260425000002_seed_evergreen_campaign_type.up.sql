-- Seed the "Evergreen" system campaign type (CON-57).
--
-- Slot conventions match the existing seed:
--   campaign type id  = Encode([5]) = uw   (continues 1..4 = Uk/gb/Ef/Vq)
--   phase ids         = Encode([50..52])    (50-block; existing types use 10/20/30/40-blocks)

INSERT INTO campaigns_types (id, name, label, description, is_system) VALUES
    ('uw', 'evergreen', 'Evergreen',
     'Build and maintain timeless content that retains value over months and years — definitive guides, cornerstone explainers, and reference material that compounds in search and social as it ages.',
     TRUE);

INSERT INTO campaigns_types_phases (id, campaign_type_id, name, purpose, sequence) VALUES
    ('Kj', 'uw', 'Foundation',
     'Build the cornerstone pieces the campaign will rely on for years. Pillar guides (definitive, comprehensive coverage of a topic), how-to and tutorial content (step-by-step with stable steps), explainers and definitions (glossaries, "what is X" pieces), reference content (comparisons, decision frameworks, checklists), and originals worth citing (research summaries, primary-source synthesis). Optimize for search intent that does not decay (informational, navigational, comparison).',
     1),
    ('hD', 'uw', 'Distribute & Cross-Link',
     'Spread the cornerstone reach across channels and weave it into the rest of the content graph. Channel-specific adaptations (long-form blog → carousel, threads, video, slide deck), internal cross-linking from newer pieces, syndication and republishing on partner channels, search-snippet optimization (FAQs, structured data, featured-snippet phrasing), and inclusion in onboarding / sales-enablement collateral so the asset earns reuse outside marketing.',
     2),
    ('IO', 'uw', 'Refresh & Compound',
     'Keep evergreen content actually evergreen. Periodic updates (refresh statistics, swap dated examples, modernize screenshots, add new sections), audit and consolidation (merge overlapping pieces, redirect stale URLs), expansion based on what readers actually search for after landing (related-question content), and retirement of pieces that have aged past usefulness. Cadence: review every 6–12 months.',
     3);
