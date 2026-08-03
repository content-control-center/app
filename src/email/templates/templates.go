package templates

import (
	"context"
	"embed"
	"fmt"

	"github.com/ogen-app/ogen/src/models"
	"github.com/ogen-app/ogen/src/repository"
)

// Template keys. Each maps to a typed data struct + a defaults/<key>.{html,txt}.tmpl
// pair embedded below. password_reset / verify_email are reserved for the
// follow-up consumer issues (CON-154 §13) and are intentionally not seeded here.
const (
	KeyWelcome  = "welcome"
	KeyDripDay2 = "drip_day2"
	KeyDripDay5 = "drip_day5"
	KeyDripDay7 = "drip_day7"
)

// Data is the context passed to every default template. Transactional
// templates (e.g. welcome) simply ignore UnsubscribeURL, which is set only for
// marketing mail. A future template needing bespoke fields can introduce its
// own struct + rendering path.
type Data struct {
	Name           string
	WorkspaceName  string
	AppURL         string
	UnsubscribeURL string
}

//go:embed defaults/*.tmpl
var defaultFS embed.FS

// Runtime-variable descriptions. These document each [[ .Var ]] placeholder;
// they're stored on EmailTemplate.Variables and kept in sync by SeedDefaults.
// Keep them aligned with the Data struct fields.
const (
	varName           = "The recipient's display name (from their user record)."
	varWorkspaceName  = "The recipient's workspace (tenant) name."
	varAppURL         = "Absolute app URL (APP_BASE_URL) for the primary call-to-action link."
	varUnsubscribeURL = "Signed one-click unsubscribe URL; required in every marketing footer."
)

// transactionalVars / marketingVars build the per-template variable doc sets.
// Each call returns a fresh map, so specs never share (and can't mutate) state.
func transactionalVars() models.StringMap {
	return models.StringMap{
		"Name":          varName,
		"WorkspaceName": varWorkspaceName,
		"AppURL":        varAppURL,
	}
}

func marketingVars() models.StringMap {
	m := transactionalVars()
	m["UnsubscribeURL"] = varUnsubscribeURL
	return m
}

// defaultSpec pairs a key with its kind + subject line + variable docs; the
// bodies are read from the embedded defaults/<key>.{html,txt}.tmpl files.
type defaultSpec struct {
	Key       string
	Kind      models.EmailKind
	Subject   string
	Variables models.StringMap
}

var defaultSpecs = []defaultSpec{
	{KeyWelcome, models.EmailKindTransactional, "Welcome to Ogen, [[ .Name ]]", transactionalVars()},
	{KeyDripDay2, models.EmailKindMarketing, "Getting the most out of Ogen", marketingVars()},
	{KeyDripDay5, models.EmailKindMarketing, "Your content, on autopilot", marketingVars()},
	{KeyDripDay7, models.EmailKindMarketing, "A quick check-in from Ogen", marketingVars()},
}

// Defaults returns the built-in default templates, reading each body from the
// embedded files. Used by SeedDefaults.
func Defaults() ([]models.EmailTemplate, error) {
	out := make([]models.EmailTemplate, 0, len(defaultSpecs))
	for _, s := range defaultSpecs {
		html, err := defaultFS.ReadFile("defaults/" + s.Key + ".html.tmpl")
		if err != nil {
			return nil, fmt.Errorf("email templates: read %s html: %w", s.Key, err)
		}
		text, err := defaultFS.ReadFile("defaults/" + s.Key + ".txt.tmpl")
		if err != nil {
			return nil, fmt.Errorf("email templates: read %s text: %w", s.Key, err)
		}
		out = append(out, models.EmailTemplate{
			Key:       s.Key,
			Subject:   s.Subject,
			HTML:      string(html),
			Text:      string(text),
			Kind:      s.Kind,
			Variables: s.Variables,
			Version:   1,
		})
	}
	return out, nil
}

// SeedDefaults upserts-if-absent the embedded default templates into the store
// (CON-154 FR10). Idempotent: existing (operator-edited) rows are left
// untouched, so copy edits survive redeploys. Returns the count newly seeded.
func SeedDefaults(ctx context.Context, repo repository.EmailTemplateRepository) (int, error) {
	defs, err := Defaults()
	if err != nil {
		return 0, err
	}
	seeded := 0
	for i := range defs {
		created, err := repo.InsertIfAbsent(ctx, &defs[i])
		if err != nil {
			return seeded, err
		}
		if created {
			seeded++
		}
		// Keep the code-owned variable docs current even for rows seeded before
		// a variable was added; operator-edited copy is left untouched.
		if err := repo.SyncVariables(ctx, defs[i].Key, defs[i].Variables); err != nil {
			return seeded, err
		}
	}
	return seeded, nil
}
