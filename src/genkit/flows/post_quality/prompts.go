package post_quality

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed prompts/post_quality.tmpl
var promptFS embed.FS

// templates holds the parsed system and user prompt blocks. The system
// block is static (the rubric/anchors/calibration) and forms the stable,
// cacheable prefix; the user block carries the per-call campaign context,
// platform conventions, and post body.
type templates struct {
	system *template.Template
	user   *template.Template
}

// loadTemplates parses the embedded prompt file into its named blocks.
// Called once at flow init.
func loadTemplates() (*templates, error) {
	raw, err := promptFS.ReadFile("prompts/post_quality.tmpl")
	if err != nil {
		return nil, fmt.Errorf("read post_quality prompt: %w", err)
	}
	t, err := template.New("post_quality").Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse post_quality prompt: %w", err)
	}
	sys, usr := t.Lookup("system"), t.Lookup("user")
	if sys == nil || usr == nil {
		return nil, fmt.Errorf("post_quality prompt missing system or user block")
	}
	return &templates{system: sys, user: usr}, nil
}

// renderSystem renders the static system/rubric block.
func (t *templates) renderSystem() (string, error) {
	return render(t.system, nil)
}

// renderUser renders the per-call user block from the template data.
func (t *templates) renderUser(d assessmentTemplateData) (string, error) {
	return render(t.user, d)
}

func render(t *template.Template, data any) (string, error) {
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		return "", fmt.Errorf("render %s prompt: %w", t.Name(), err)
	}
	return b.String(), nil
}
