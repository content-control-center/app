package imagegen

import (
	"embed"
	"strings"
	"text/template"
)

//go:embed prompts/imagine.tmpl
var promptFS embed.FS

// PromptData drives the reference-first prompt scaffold (§6, Appendix A.1).
// Naming the reference ranges ("images 1–N", "final image") is what binds each
// image blob to its role — brand style vs post subject — because ordering alone
// is not enough once several references are present. The size line is secondary
// reinforcement of §7; the authoritative size comes from the ImageConfig.
type PromptData struct {
	Platform    string // target platform, e.g. "Instagram"
	BrandCount  int    // number of BRAND STYLE reference images
	HasSubject  bool   // whether a POST SUBJECT reference is attached
	SubjectDesc string // short description of the subject to preserve
	Instruction string // caller's extra instruction (may be empty)
	AspectRatio string // secondary, prompt-level reinforcement of §7
	Resolution  string // secondary, prompt-level reinforcement of §7
}

// promptTmpl mirrors Appendix A.1; the BRAND STYLE and POST SUBJECT blocks are
// conditional so the scaffold degrades cleanly when a generation supplies only
// a prompt, only brand refs, or only a subject.
var promptTmpl = template.Must(template.New("imagine").Parse(loadPrompt()))

// loadPrompt reads the embedded template. go:embed guarantees the file is
// present at build time, so a read failure is impossible in a built binary.
func loadPrompt() string {
	b, _ := promptFS.ReadFile("prompts/imagine.tmpl")
	return strings.TrimSpace(string(b))
}

// Render produces the prompt text for a generation request.
func Render(d PromptData) (string, error) {
	var b strings.Builder
	if err := promptTmpl.Execute(&b, d); err != nil {
		return "", err
	}
	return b.String(), nil
}
