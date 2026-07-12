package imagegen

import (
	"context"
	"fmt"

	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
)

// ImagineFlow is the singleton Genkit flow, registered for Dev-UI discovery.
// The async River image_generate job invokes generation through the callback
// from NewImagineCallback rather than this wrapped Flow.
var ImagineFlow *core.Flow[ImagineRequest, *ImagineResponse, struct{}]

// imagineRunner is the direct closure the callback delegates to. Set by
// InitImagine.
var imagineRunner func(ctx context.Context, req ImagineRequest) (*ImagineResponse, error)

// InitImagine registers the imagine Genkit flow on g. cfg.Model must already be
// built (server wiring, via NewModel). g is used only for flow
// registration/tracing — the model call runs on the reloadable Model's own
// genkit instance, so this can share the app's primary genkit instance without
// coupling to the Gemini key lifecycle.
func InitImagine(g *genkit.Genkit, cfg ImagineFlowConfig) error {
	if cfg.Model == nil {
		return fmt.Errorf("imagegen: InitImagine requires a non-nil Model")
	}

	ImagineFlow = genkit.DefineFlow(g, "imagine",
		func(ctx context.Context, req ImagineRequest) (*ImagineResponse, error) {
			return runImagine(ctx, req, cfg)
		},
	)

	imagineRunner = func(ctx context.Context, req ImagineRequest) (*ImagineResponse, error) {
		return runImagine(ctx, req, cfg)
	}

	return nil
}

// NewImagineCallback returns the callback the River worker invokes. This flow
// is not streaming, so there is no OnEventFunc.
func NewImagineCallback() func(ctx context.Context, req ImagineRequest) (*ImagineResponse, error) {
	return func(ctx context.Context, req ImagineRequest) (*ImagineResponse, error) {
		return imagineRunner(ctx, req)
	}
}
