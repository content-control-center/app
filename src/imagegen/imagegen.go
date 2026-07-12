package imagegen

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"google.golang.org/genai"

	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/secrets"
	"github.com/ogen-app/ogen/src/usage"
	"github.com/ogen-app/ogen/src/vendors/llm"
)

const (
	backendDeveloper = "developer" // global Gemini Developer API (v1)
	backendVertex    = "vertex"    // Vertex AI EU endpoint (CON-109, deferred)
)

// ErrUnavailable is returned by Generate when no gemini_api_key is currently
// configured (developer backend). Like embedding, the key can be set / rotated
// via the secrets API without a restart (CON-104), so a later call can succeed.
var ErrUnavailable = errors.New("imagegen: no gemini_api_key configured")

// ReferenceImage is a decoded reference image passed as a multimodal part.
type ReferenceImage struct {
	MIMEType string // e.g. "image/png"
	Data     []byte
}

// Request is a single image-generation call. BrandRefs supply the style role
// and are ordered first (a stable, cacheable prefix); Subject supplies the
// content role and is placed last (§6). Premium selects the Pro model tier.
type Request struct {
	Platform    string           // target platform, for the prompt scaffold
	SubjectDesc string           // short description of the subject to preserve
	Instruction string           // caller's extra instruction (optional)
	BrandRefs   []ReferenceImage // brand-kit style references (optional)
	Subject     *ReferenceImage  // post subject reference (optional)
	AspectRatio string           // authoritative output ratio (§7); required
	Resolution  string           // authoritative output resolution (§7); "" -> 1K
	Premium     bool             // use the Pro model tier
	Feature     string           // usage-event feature tag; "" -> "post_image"
}

// Result is a completed generation. AspectRatio/Resolution/Model are echoed
// back for the caller to persist on the Asset for reproducibility (§7). The
// output carries SynthID + C2PA content credentials inherently (§10); callers
// record that provenance on the Asset.
type Result struct {
	Image       []byte
	MIMEType    string
	Model       string
	AspectRatio string
	Resolution  string
}

// imagePlugin is the slice of the googlegenai plugin surface this package uses:
// a genkit plugin that can define a model. Both *googlegenai.GoogleAI (v1) and
// *googlegenai.VertexAI (CON-109) satisfy it, so the backend is a config switch
// with no change to the request/response contract.
type imagePlugin interface {
	api.Plugin
	DefineModel(g *genkit.Genkit, name string, opts *ai.ModelOptions) (ai.Model, error)
}

// live holds the current genkit instance and defined model handles. The whole
// triple is swapped atomically on a key change so a rebuild never leaves a
// model handle bound to a stale registry.
type live struct {
	g       *genkit.Genkit
	def     ai.Model
	premium ai.Model
}

// Service generates images through the Genkit googlegenai plugin. Its backing
// models are rebuilt when gemini_api_key is set / rotated / cleared (developer
// backend); the Service reference is stable for the process lifetime.
type Service struct {
	backend        string
	defaultModel   string
	premiumModel   string
	maxRefs        int
	vertexProject  string
	vertexLocation string

	secretStore secrets.Store
	recorder    *usage.Recorder

	mu      sync.RWMutex
	current *live // nil = unavailable
}

// New builds the image-generation service and performs the initial model build.
// When gemini_api_key is unset the service is "unavailable": Generate returns
// ErrUnavailable until the key is added via the secrets API (no restart).
func New(ctx context.Context, cfg *config.Config, secretStore secrets.Store, recorder *usage.Recorder) (*Service, error) {
	s := &Service{
		backend:        cfg.ImageGenBackend,
		defaultModel:   cfg.ImageModelID,
		premiumModel:   cfg.ImagePremiumModelID,
		maxRefs:        cfg.ImageMaxReferences,
		vertexProject:  cfg.VertexProjectID,
		vertexLocation: cfg.VertexLocation,
		secretStore:    secretStore,
		recorder:       recorder,
	}

	if err := s.rebuild(ctx); err != nil {
		return nil, err
	}

	// The developer backend authenticates with gemini_api_key, so it reloads on
	// key changes. The vertex backend uses ADC (CON-109) — no key to subscribe.
	if s.backend != backendVertex {
		secretStore.Subscribe(secrets.NameGeminiAPIKey, func() {
			if err := s.rebuild(context.Background()); err != nil {
				slog.Error("imagegen rebuild after gemini_api_key change failed",
					logging.AttrComponent, "imagegen", logging.AttrError, err)
			}
		})
	}

	return s, nil
}

// Available reports whether a generation can currently be served.
func (s *Service) Available() bool { return s.snapshot() != nil }

// Generate validates, dispatches, meters, and extracts a single image. Size is
// validated before any network work so an unsupported ratio/resolution fails
// fast with a caller-facing reason (§7).
func (s *Service) Generate(ctx context.Context, req Request) (*Result, error) {
	modelID := s.defaultModel
	if req.Premium {
		modelID = s.premiumModel
	}

	ratio := req.AspectRatio
	if ratio == "" {
		return nil, fmt.Errorf("imagegen: aspect ratio is required")
	}
	res := req.Resolution
	if res == "" {
		res = DefaultResolution
	}
	if err := ValidateSize(modelID, ratio, res); err != nil {
		return nil, fmt.Errorf("imagegen: %w", err)
	}

	refCount := len(req.BrandRefs)
	if req.Subject != nil {
		refCount++
	}
	if refCount > s.maxRefs {
		return nil, fmt.Errorf("imagegen: %d reference images exceed the per-request limit of %d", refCount, s.maxRefs)
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
		return nil, err
	}

	// Prompt text, then brand refs (stable prefix), then subject last (A.2/§6).
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
	cfg := &genai.GenerateContentConfig{
		ResponseModalities: []string{"TEXT", "IMAGE"},
		ImageConfig:        &genai.ImageConfig{AspectRatio: ratio, ImageSize: res},
		CandidateCount:     1,
	}

	cur := s.snapshot()
	if cur == nil {
		return nil, ErrUnavailable
	}
	model := cur.def
	if req.Premium {
		model = cur.premium
	}

	resp, err := genkit.Generate(ctx, cur.g,
		ai.WithModel(model),
		ai.WithMessages(ai.NewUserMessage(parts...)),
		ai.WithConfig(cfg),
	)
	if err != nil {
		return nil, fmt.Errorf("imagegen: generate (%s): %w", modelID, err)
	}

	// Metering: the gemini meter tags this operation_type "generate_image" (§9).
	// Requires a tenant in ctx (the River worker sets it); untenanted calls are
	// silently skipped by the recorder.
	feature := req.Feature
	if feature == "" {
		feature = "post_image"
	}
	s.recorder.RecordResp(ctx, llm.VendorGemini, modelID, feature, resp)

	img, mime, err := extractImage(resp)
	if err != nil {
		return nil, err
	}

	return &Result{
		Image:       img,
		MIMEType:    mime,
		Model:       modelID,
		AspectRatio: ratio,
		Resolution:  res,
	}, nil
}

// rebuild resolves the current backend/key and swaps in a fresh genkit instance
// with the two image models defined. A missing key (developer backend) leaves
// the service unavailable rather than erroring.
func (s *Service) rebuild(ctx context.Context) error {
	plugin, available, err := s.buildPlugin(ctx)
	if err != nil {
		s.swap(nil)
		return err
	}
	if !available {
		s.swap(nil)
		slog.InfoContext(ctx, "gemini_api_key not configured; image generation disabled",
			logging.AttrComponent, "imagegen")
		return nil
	}

	// A fresh genkit instance per rebuild: the plugin captures its credentials
	// at construction and re-defining a model of the same name on one instance
	// conflicts, so each rebuild gets its own instance (mirrors embedding.go).
	g := genkit.Init(ctx, genkit.WithPlugins(plugin))

	def, err := defineImageModel(plugin, g, s.defaultModel)
	if err != nil {
		s.swap(nil)
		return fmt.Errorf("imagegen: define model %q: %w", s.defaultModel, err)
	}
	premium := def
	if s.premiumModel != s.defaultModel {
		premium, err = defineImageModel(plugin, g, s.premiumModel)
		if err != nil {
			s.swap(nil)
			return fmt.Errorf("imagegen: define model %q: %w", s.premiumModel, err)
		}
	}

	s.swap(&live{g: g, def: def, premium: premium})
	slog.InfoContext(ctx, "image generation ready",
		logging.AttrComponent, "imagegen", "backend", s.backend,
		"default", s.defaultModel, "premium", s.premiumModel)
	return nil
}

// buildPlugin constructs the backend plugin. The second return reports whether
// the service is available (developer backend with no key -> false, no error).
func (s *Service) buildPlugin(ctx context.Context) (imagePlugin, bool, error) {
	switch s.backend {
	case backendVertex:
		// CON-109: EU residency via Vertex AI + ADC. Wired so the switch is
		// config-only; unvalidated until that work lands.
		return &googlegenai.VertexAI{ProjectID: s.vertexProject, Location: s.vertexLocation}, true, nil
	case backendDeveloper, "":
		key, err := s.secretStore.Get(ctx, secrets.NameGeminiAPIKey)
		if errors.Is(err, secrets.ErrNotFound) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("imagegen: read gemini_api_key: %w", err)
		}
		return &googlegenai.GoogleAI{APIKey: key}, true, nil
	default:
		return nil, false, fmt.Errorf("imagegen: unknown IMAGE_GEN_BACKEND %q", s.backend)
	}
}

// defineImageModel registers an image model with multimodal support. The image
// model ids are not in the plugin's built-in known list, so they must be
// defined explicitly (mirrors DefineEmbedder in embedding.go).
func defineImageModel(p imagePlugin, g *genkit.Genkit, id string) (ai.Model, error) {
	return p.DefineModel(g, id, &ai.ModelOptions{
		Label:    id,
		Supports: &ai.ModelSupports{Media: true, Multiturn: true},
	})
}

func (s *Service) snapshot() *live {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func (s *Service) swap(l *live) {
	s.mu.Lock()
	s.current = l
	s.mu.Unlock()
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
