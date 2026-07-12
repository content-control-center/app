package imagegen

import (
	"fmt"
	"strings"
	"text/template"
)

// PromptData drives the reference-first prompt scaffold (§6, Appendix A.1).
// Naming the reference ranges ("images 1–N", "final image") is what binds each
// image blob to its role — brand style vs post subject — because ordering alone
// is not enough once several references are present.
type PromptData struct {
	Platform    string // target platform, e.g. "Instagram"
	BrandCount  int    // number of BRAND STYLE reference images
	HasSubject  bool   // whether a POST SUBJECT reference is attached
	SubjectDesc string // short description of the subject to preserve
	Instruction string // caller's extra instruction (may be empty)
	AspectRatio string // secondary, prompt-level reinforcement of §7
	Resolution  string // secondary, prompt-level reinforcement of §7
}

// promptTmpl mirrors Appendix A.1. The BRAND STYLE and POST SUBJECT blocks are
// conditional so the scaffold degrades cleanly when a generation supplies only
// a prompt, only brand refs, or only a subject. The size line is the secondary
// prompt-level reinforcement of §7 — the authoritative size still comes from
// the ImageConfig on the request.
var promptTmpl = template.Must(template.New("imagegen").Parse(strings.TrimSpace(`
You are generating a branded social image for {{.Platform}}.
{{if or (gt .BrandCount 0) .HasSubject}}
Reference images are provided in this order:
{{- if gt .BrandCount 0}}
- BRAND STYLE (images 1{{if gt .BrandCount 1}}–{{.BrandCount}}{{end}}): the visual identity to
  emulate — color palette, texture, lighting, typography feel, overall
  aesthetic. Do NOT reuse their subject matter or composition.
{{- end}}
{{- if .HasSubject}}
- POST SUBJECT (final image): the subject of this post{{if .SubjectDesc}} — {{.SubjectDesc}}{{end}}.
  Preserve its identity and defining details.
{{- end}}
{{end}}
Task: {{if .HasSubject}}Create a new image depicting the POST SUBJECT, rendered{{else}}Create a new image rendered{{end}}
{{- if gt .BrandCount 0}} entirely in the visual style of the BRAND STYLE references.{{else}}.{{end}}
{{- if .Instruction}} {{.Instruction}}{{end}}

Constraints: aspect ratio {{.AspectRatio}}, resolution {{.Resolution}}, high
fidelity{{if gt .BrandCount 0}}, faithful brand color. Do not add logos or watermarks not present in the references.{{else}}.{{end}}
`)))

// Render produces the prompt text for a generation request.
func Render(d PromptData) (string, error) {
	var b strings.Builder
	if err := promptTmpl.Execute(&b, d); err != nil {
		return "", fmt.Errorf("imagegen: render prompt: %w", err)
	}
	return b.String(), nil
}
