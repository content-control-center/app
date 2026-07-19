package imagegen

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/firebase/genkit/go/ai"
	"google.golang.org/genai"

	"github.com/ogen-app/ogen/src/vendors/llm"
)

// runImagine validates, meters, dispatches, and extracts a single image. Size
// is validated before any paid work so an unsupported ratio/resolution fails
// fast with a caller-facing reason (§7).
func runImagine(ctx context.Context, req ImagineRequest, cfg ImagineFlowConfig) (*ImagineResponse, error) {
	if cfg.Model == nil || !cfg.Model.Available() {
		return nil, &AIError{Msg: "image generation is unavailable (no gemini_api_key configured)"}
	}

	modelID := cfg.Model.ModelIDFor(req.Premium)

	// ── Validate input + output size (§7) ───────────────────────────────────
	ratio := req.AspectRatio
	if ratio == "" {
		return nil, &ValidationError{Msg: "aspect ratio is required"}
	}
	res := req.Resolution
	if res == "" {
		res = DefaultResolution
	}
	if err := ValidateSize(modelID, ratio, res); err != nil {
		return nil, &ValidationError{Msg: err.Error()}
	}

	refCount := len(req.BrandRefs)
	if req.Subject != nil {
		refCount++
	}
	if limit := cfg.Model.MaxReferences(); refCount > limit {
		return nil, &ValidationError{Msg: fmt.Sprintf("%d reference images exceed the per-request limit of %d", refCount, limit)}
	}

	// ── Enforcement gate (CON-86 FR9). Nil checker = no gate; fails open. ────
	// Placed before the first paid call.
	if err := cfg.Checker.Enforce(ctx); err != nil {
		return nil, err // *usage.LimitExceededError when blocked
	}

	prompt, err := Render(PromptData{
		Platform:    req.Platform,
		BrandCount:  len(req.BrandRefs),
		HasSubject:  req.Subject != nil,
		SubjectDesc: req.SubjectDesc,
		Instruction: req.Instruction,
		AspectRatio: ratio,
		Resolution:  res,
	})
	if err != nil {
		return nil, fmt.Errorf("imagegen: render prompt: %w", err)
	}

	// Prompt text, then brand refs (stable, cacheable prefix), then subject
	// last (A.2/§6).
	parts := make([]*ai.Part, 0, refCount+1)
	parts = append(parts, ai.NewTextPart(prompt))
	for _, r := range req.BrandRefs {
		parts = append(parts, mediaPart(r))
	}
	if req.Subject != nil {
		parts = append(parts, mediaPart(*req.Subject))
	}

	// ImageConfig is authoritative for output size (§7); the prompt only echoes
	// it as secondary reinforcement.
	gcfg := &genai.GenerateContentConfig{
		ResponseModalities: []string{"TEXT", "IMAGE"},
		ImageConfig:        &genai.ImageConfig{AspectRatio: ratio, ImageSize: res},
		CandidateCount:     1,
	}

	resp, usedModel, err := cfg.Model.Generate(ctx, parts, req.Premium, gcfg)
	if err != nil {
		return nil, &AIError{Msg: fmt.Sprintf("model call failed (%s): %v", usedModel, err)}
	}

	// ── Metering (CON-86 FR1). The gemini meter tags operation "generate_image"
	// (§9). Requires a tenant in ctx (the River worker sets it); untenanted
	// calls are silently skipped by the recorder. Nil recorder = no-op. ──────
	feature := cfg.Feature
	if feature == "" {
		feature = "post_image"
	}
	cfg.Recorder.RecordResp(ctx, llm.VendorGemini, usedModel, feature, resp)

	img, mime, err := extractImage(resp)
	if err != nil {
		return nil, &AIError{Msg: err.Error()}
	}

	return &ImagineResponse{
		Image:       img,
		MIMEType:    mime,
		Model:       usedModel,
		AspectRatio: ratio,
		Resolution:  res,
	}, nil
}

func mediaPart(r ReferenceImage) *ai.Part {
	return ai.NewMediaPart(r.MIMEType, "data:"+r.MIMEType+";base64,"+base64.StdEncoding.EncodeToString(r.Data))
}

// extractImage pulls the first image out of the multi-part response defensively:
// the model returns commentary text alongside the image, so parts are iterated
// rather than assuming a single part (A.3). Model commentary is surfaced in the
// error when no image comes back (e.g. a safety refusal).
func extractImage(resp *ai.ModelResponse) ([]byte, string, error) {
	if resp == nil || resp.Message == nil {
		return nil, "", errNoImage("")
	}
	var commentary string
	for _, p := range resp.Message.Content {
		if p.IsMedia() {
			if b, mime, err := decodeDataURL(p); err == nil {
				return b, mime, nil
			}
			continue
		}
		if p.IsText() && commentary == "" {
			commentary = strings.TrimSpace(p.Text)
		}
	}
	return nil, "", errNoImage(commentary)
}

func errNoImage(commentary string) error {
	if commentary != "" {
		if len(commentary) > 200 {
			commentary = commentary[:200] + "…"
		}
		return fmt.Errorf("imagegen: response contained no image (model said: %s)", commentary)
	}
	return fmt.Errorf("imagegen: response contained no image")
}

// decodeDataURL decodes the "data:<mime>;base64,<...>" media part the plugin
// emits for a generated image (gemini.go translateCandidate -> ai.NewMediaPart).
func decodeDataURL(p *ai.Part) ([]byte, string, error) {
	const pfx = "data:"
	s := p.Text
	if !strings.HasPrefix(s, pfx) {
		return nil, "", fmt.Errorf("media part is not a data URL")
	}
	comma := strings.IndexByte(s, ',')
	if comma < 0 {
		return nil, "", fmt.Errorf("malformed data URL")
	}
	mime := p.ContentType
	if mime == "" {
		mime = strings.SplitN(s[len(pfx):comma], ";", 2)[0]
	}
	b, err := base64.StdEncoding.DecodeString(s[comma+1:])
	if err != nil {
		return nil, "", fmt.Errorf("base64 decode image: %w", err)
	}
	return b, mime, nil
}
