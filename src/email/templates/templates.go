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

// defaultSpec pairs a key with its kind + subject line; the bodies are read
// from the embedded defaults/<key>.{html,txt}.tmpl files.
type defaultSpec struct {
	Key     string
	Kind    models.EmailKind
	Subject string
}

var defaultSpecs = []defaultSpec{
	{KeyWelcome, models.EmailKindTransactional, "Welcome to Ogen, [[ .Name ]]"},
	{KeyDripDay2, models.EmailKindMarketing, "Getting the most out of Ogen"},
	{KeyDripDay5, models.EmailKindMarketing, "Your content, on autopilot"},
	{KeyDripDay7, models.EmailKindMarketing, "A quick check-in from Ogen"},
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
			Key:     s.Key,
			Subject: s.Subject,
			HTML:    string(html),
			Text:    string(text),
			Kind:    s.Kind,
			Version: 1,
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
	}
	return seeded, nil
}
