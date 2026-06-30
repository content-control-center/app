package llm

// Role selects which configured model a flow wants. content_plan,
// post_assistant, and enrich_brief use generation; post_quality uses quality.
type Role string

const (
	RoleGeneration Role = "generation"
	RoleQuality    Role = "quality"
)

// Provider resolves foundation-model references by role so flows don't
// hardcode the "anthropic/" prefix or carry the configured model ids
// themselves (CON-86 FR12). Constructed once from config and shared.
type Provider struct {
	generationModel string
	qualityModel    string
}

// NewProvider builds a Provider from the configured model ids
// (cfg.ModelID, cfg.QualityModelID).
func NewProvider(generationModel, qualityModel string) *Provider {
	return &Provider{generationModel: generationModel, qualityModel: qualityModel}
}

// Model returns the bare model id for a role — the value recorded as the
// `model` dimension and looked up in the price map.
func (p *Provider) Model(role Role) string {
	if role == RoleQuality {
		return p.qualityModel
	}
	return p.generationModel
}

// Ref returns the genkit "provider/model" reference for ai.WithModelName,
// e.g. "anthropic/claude-sonnet-4-5-20250929".
func (p *Provider) Ref(role Role) string {
	return VendorAnthropic + "/" + p.Model(role)
}
