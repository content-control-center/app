package imagegen

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"google.golang.org/genai"

	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/logging"
	"github.com/ogen-app/ogen/src/secrets"
)

const (
	backendDeveloper = "developer" // global Gemini Developer API (v1)
	backendVertex    = "vertex"    // Vertex AI EU endpoint (CON-109, deferred)
)

// ErrUnavailable is returned by Generate when no gemini_api_key is currently
// configured (developer backend). Like the embedder, the key can be set /
// rotated via the secrets API without a restart (CON-104), so a later call can
// succeed.
var ErrUnavailable = errors.New("imagegen: no gemini_api_key configured")

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

// Model is the reloadable Gemini image-model provider. Its backing genkit
// instance + model handles are rebuilt when gemini_api_key is set / rotated /
// cleared (developer backend); the Model reference is stable for the process
// lifetime, mirroring the reloadable embedder in src/embedding. It is separate
// from the shared Anthropic flow instance so a Gemini key change never disturbs
// the text flows (and vice-versa).
type Model struct {
	backend        string
	defaultModel   string
	premiumModel   string
	maxRefs        int
	vertexProject  string
	vertexLocation string

	secretStore secrets.Store

	mu      sync.RWMutex
	current *live // nil = unavailable
}

// NewModel builds the reloadable image model and performs the initial build.
// When gemini_api_key is unset the model is "unavailable" (Available reports
// false, Generate returns ErrUnavailable) until the key is added via the
// secrets API — no restart. The returned *Model is always non-nil on success.
func NewModel(ctx context.Context, cfg *config.Config, secretStore secrets.Store) (*Model, error) {
	m := &Model{
		backend:        cfg.ImageGenBackend,
		defaultModel:   cfg.ImageModelID,
		premiumModel:   cfg.ImagePremiumModelID,
		maxRefs:        cfg.ImageMaxReferences,
		vertexProject:  cfg.VertexProjectID,
		vertexLocation: cfg.VertexLocation,
		secretStore:    secretStore,
	}

	if err := m.rebuild(ctx); err != nil {
		return nil, err
	}

	// The developer backend authenticates with gemini_api_key, so it reloads on
	// key changes. The vertex backend uses ADC (CON-109) — no key to subscribe.
	if m.backend != backendVertex {
		secretStore.Subscribe(secrets.NameGeminiAPIKey, func() {
			if err := m.rebuild(context.Background()); err != nil {
				slog.Error("imagegen rebuild after gemini_api_key change failed",
					logging.AttrComponent, "imagegen", logging.AttrError, err)
			}
		})
	}

	return m, nil
}

// Available reports whether a generation can currently be served.
func (m *Model) Available() bool { return m.snapshot() != nil }

// ModelIDFor returns the model id for a tier — the value validated against the
// §7 size table and recorded as the usage `model` dimension.
func (m *Model) ModelIDFor(premium bool) string {
	if premium {
		return m.premiumModel
	}
	return m.defaultModel
}

// MaxReferences is the per-request reference-image cap (§6).
func (m *Model) MaxReferences() int { return m.maxRefs }

// Generate resolves the current model for the tier and issues one generation,
// returning the raw genkit response (for metering) and the model id used. The
// call runs on the Model's own reloadable genkit instance.
func (m *Model) Generate(ctx context.Context, parts []*ai.Part, premium bool, gcfg *genai.GenerateContentConfig) (*ai.ModelResponse, string, error) {
	cur := m.snapshot()
	if cur == nil {
		return nil, m.ModelIDFor(premium), ErrUnavailable
	}
	model := cur.def
	if premium {
		model = cur.premium
	}
	resp, err := genkit.Generate(ctx, cur.g,
		ai.WithModel(model),
		ai.WithMessages(ai.NewUserMessage(parts...)),
		ai.WithConfig(gcfg),
	)
	return resp, m.ModelIDFor(premium), err
}

// rebuild resolves the current backend/key and swaps in a fresh genkit instance
// with the two image models defined. A missing key (developer backend) leaves
// the model unavailable rather than erroring.
func (m *Model) rebuild(ctx context.Context) error {
	plugin, available, err := m.buildPlugin(ctx)
	if err != nil {
		m.swap(nil)
		return err
	}
	if !available {
		m.swap(nil)
		slog.InfoContext(ctx, "gemini_api_key not configured; image generation disabled",
			logging.AttrComponent, "imagegen")
		return nil
	}

	// A fresh genkit instance per rebuild: the plugin captures its credentials
	// at construction and re-defining a model of the same name on one instance
	// conflicts, so each rebuild gets its own instance (mirrors embedding.go).
	g := genkit.Init(ctx, genkit.WithPlugins(plugin))

	def, err := defineImageModel(plugin, g, m.defaultModel)
	if err != nil {
		m.swap(nil)
		return fmt.Errorf("imagegen: define model %q: %w", m.defaultModel, err)
	}
	premium := def
	if m.premiumModel != m.defaultModel {
		premium, err = defineImageModel(plugin, g, m.premiumModel)
		if err != nil {
			m.swap(nil)
			return fmt.Errorf("imagegen: define model %q: %w", m.premiumModel, err)
		}
	}

	m.swap(&live{g: g, def: def, premium: premium})
	slog.InfoContext(ctx, "image generation ready",
		logging.AttrComponent, "imagegen", "backend", m.backend,
		"default", m.defaultModel, "premium", m.premiumModel)
	return nil
}

// buildPlugin constructs the backend plugin. The second return reports whether
// the model is available (developer backend with no key -> false, no error).
func (m *Model) buildPlugin(ctx context.Context) (imagePlugin, bool, error) {
	switch m.backend {
	case backendVertex:
		// CON-109: EU residency via Vertex AI + ADC. Wired so the switch is
		// config-only; unvalidated until that work lands.
		return &googlegenai.VertexAI{ProjectID: m.vertexProject, Location: m.vertexLocation}, true, nil
	case backendDeveloper, "":
		key, err := m.secretStore.Get(ctx, secrets.NameGeminiAPIKey)
		if errors.Is(err, secrets.ErrNotFound) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("imagegen: read gemini_api_key: %w", err)
		}
		return &googlegenai.GoogleAI{APIKey: key}, true, nil
	default:
		return nil, false, fmt.Errorf("imagegen: unknown IMAGE_GEN_BACKEND %q", m.backend)
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

func (m *Model) snapshot() *live {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

func (m *Model) swap(l *live) {
	m.mu.Lock()
	m.current = l
	m.mu.Unlock()
}
