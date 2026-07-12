package server

import (
	"context"

	"github.com/firebase/genkit/go/genkit"

	"github.com/ogen-app/ogen/src/config"
	"github.com/ogen-app/ogen/src/genkit/flows/imagegen"
	"github.com/ogen-app/ogen/src/secrets"
	"github.com/ogen-app/ogen/src/usage"
)

// imagineCallback is the function the image_generate worker invokes.
type imagineCallback = func(context.Context, imagegen.ImagineRequest) (*imagegen.ImagineResponse, error)

// initImageGen builds the reloadable Gemini image model and registers the
// imagine flow (CON-105). It mirrors initEmbedding: a dedicated Genkit instance
// hosts the flow while the reloadable Model owns the gemini_api_key lifecycle,
// so a Gemini key change never disturbs the Anthropic flow instance (and
// vice-versa). Returns the worker callback and the Model (whose Available gate
// the handler checks). When no key is configured the Model is simply
// unavailable — generation stays dormant until a key is set via the secrets
// API, no restart (CON-104).
func initImageGen(ctx context.Context, cfg *config.Config, secretStore secrets.Store, recorder *usage.Recorder, checker *usage.Checker) (imagineCallback, *imagegen.Model, error) {
	model, err := imagegen.NewModel(ctx, cfg, secretStore)
	if err != nil {
		return nil, nil, err
	}
	g := genkit.Init(ctx)
	if err := imagegen.InitImagine(g, imagegen.ImagineFlowConfig{
		Model:    model,
		Recorder: recorder,
		Checker:  checker,
	}); err != nil {
		return nil, nil, err
	}
	return imagegen.NewImagineCallback(), model, nil
}
