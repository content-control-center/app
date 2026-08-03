-- CON-154: document each email template's runtime variables. Key = the
-- [[ .Var ]] placeholder name; value = a human explanation. This is code-owned
-- metadata (source of truth is email/templates.SeedDefaults) kept in sync by
-- the boot seeder, so a fresh '{}' here is backfilled on the next boot.
ALTER TABLE email_templates
    ADD COLUMN variables jsonb NOT NULL DEFAULT '{}';
