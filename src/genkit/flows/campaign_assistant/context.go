package campaign_assistant

import (
	"bytes"
	"text/template"

	"github.com/ogen-app/ogen/src/models"
)

// assistantContext holds the rendered prompts ready for the model call.
type assistantContext struct {
	SystemPrompt string // stable system instructions (cacheable prefix)
	ContextBlock string // the campaign's current brief (stable within a turn)
}

// contextTemplateData is the view model for the prompt template. It carries
// only fields already loaded on the campaign, so context assembly costs no
// extra query — the planner stays fast (CON-112 execution-time optimisation).
type contextTemplateData struct {
	CampaignName   string
	Status         string
	CampaignType   string
	Language       string
	Description    string
	TargetPersona  string
	KeyMessages    string
	ToneGuidelines string
}

// assembleContext renders the system + context blocks from the campaign. The
// brief is placed in the context block so grounded Q&A ("summarise the brief")
// needs no tool round-trip. The rendered strings are deterministic, so an
// unchanged brief produces an identical prefix and Anthropic prompt caching
// still applies without an in-process cache.
func assembleContext(campaign *models.Campaign, systemTmpl, contextTmpl *template.Template) (*assistantContext, error) {
	campaignType := campaign.CampaignTypeID
	if campaign.CampaignType != nil && campaign.CampaignType.Name != "" {
		campaignType = campaign.CampaignType.Name
	}

	data := contextTemplateData{
		CampaignName:   campaign.Name,
		Status:         string(campaign.Status),
		CampaignType:   campaignType,
		Language:       campaign.Language,
		Description:    campaign.Description,
		TargetPersona:  campaign.TargetPersona,
		KeyMessages:    campaign.KeyMessages,
		ToneGuidelines: campaign.ToneGuidelines,
	}

	systemPrompt, err := renderTemplate(systemTmpl, data)
	if err != nil {
		return nil, err
	}
	contextBlock, err := renderTemplate(contextTmpl, data)
	if err != nil {
		return nil, err
	}
	return &assistantContext{SystemPrompt: systemPrompt, ContextBlock: contextBlock}, nil
}

func renderTemplate(tmpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
