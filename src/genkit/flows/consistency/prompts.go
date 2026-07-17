package consistency

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/ogen-app/ogen/src/models"
)

//go:embed prompts/consistency.tmpl
var promptFS embed.FS

// maxPostBodyChars bounds each post's body in the posts-review prompt so a large
// campaign stays within a single model call.
const maxPostBodyChars = 600

// templates holds the parsed prompt blocks for both reviews. The system blocks
// are static (the stable, cacheable prefix); the user blocks carry the per-call
// campaign brief, phases, and post bodies.
type templates struct {
	briefSystem *template.Template
	briefUser   *template.Template
	postsSystem *template.Template
	postsUser   *template.Template
}

// loadTemplates parses the embedded prompt file into its named blocks. Called
// once at flow init.
func loadTemplates() (*templates, error) {
	raw, err := promptFS.ReadFile("prompts/consistency.tmpl")
	if err != nil {
		return nil, fmt.Errorf("read consistency prompt: %w", err)
	}
	t, err := template.New("consistency").Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse consistency prompt: %w", err)
	}
	tt := &templates{
		briefSystem: t.Lookup("brief_system"),
		briefUser:   t.Lookup("brief_user"),
		postsSystem: t.Lookup("posts_system"),
		postsUser:   t.Lookup("posts_user"),
	}
	if tt.briefSystem == nil || tt.briefUser == nil || tt.postsSystem == nil || tt.postsUser == nil {
		return nil, fmt.Errorf("consistency prompt missing a required block")
	}
	return tt, nil
}

// renderBriefSystem renders the static brief-review rubric block.
func (t *templates) renderBriefSystem() (string, error) { return render(t.briefSystem, nil) }

// renderBriefUser renders the per-call brief-review block from the campaign.
func (t *templates) renderBriefUser(c *models.Campaign) (string, error) {
	return render(t.briefUser, briefPromptData(c))
}

// renderPostsSystem renders the static posts-review rubric block.
func (t *templates) renderPostsSystem() (string, error) { return render(t.postsSystem, nil) }

// renderPostsUser renders the per-call posts-review block from the campaign and
// the posts under review (each body truncated to maxPostBodyChars).
func (t *templates) renderPostsUser(c *models.Campaign, posts []models.Post) (string, error) {
	return render(t.postsUser, postsPromptData(c, posts))
}

func render(t *template.Template, data any) (string, error) {
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		return "", fmt.Errorf("render %s prompt: %w", t.Name(), err)
	}
	return b.String(), nil
}

// briefPromptData assembles the brief-review template data from the campaign.
func briefPromptData(c *models.Campaign) briefTemplateData {
	d := briefTemplateData{
		CampaignName:   strings.TrimSpace(c.Name),
		Language:       c.Language,
		Description:    strings.TrimSpace(c.Description),
		TargetPersona:  strings.TrimSpace(c.TargetPersona),
		KeyMessages:    strings.TrimSpace(c.KeyMessages),
		ToneGuidelines: strings.TrimSpace(c.ToneGuidelines),
	}
	if c.CampaignType != nil {
		label := c.CampaignType.Label
		if label == "" {
			label = c.CampaignType.Name
		}
		d.CampaignType = strings.TrimSpace(label)
		d.CampaignGoal = strings.TrimSpace(c.CampaignType.Description)
		for _, p := range c.CampaignType.Phases {
			d.Phases = append(d.Phases, phaseTemplateData{Sequence: p.Sequence, Name: p.Name, Purpose: p.Purpose})
		}
	}
	return d
}

// postsPromptData assembles the posts-review template data, truncating each body.
func postsPromptData(c *models.Campaign, posts []models.Post) postsTemplateData {
	d := postsTemplateData{
		TargetPersona:  strings.TrimSpace(c.TargetPersona),
		KeyMessages:    strings.TrimSpace(c.KeyMessages),
		ToneGuidelines: strings.TrimSpace(c.ToneGuidelines),
		Language:       c.Language,
	}
	for _, p := range posts {
		d.Posts = append(d.Posts, postTemplateData{
			ID:       p.ID,
			Title:    strings.TrimSpace(p.Title),
			PostType: p.PlatformPostType,
			Body:     truncate(p.Content, maxPostBodyChars),
		})
	}
	return d
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}
