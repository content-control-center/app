// Package imagegen generates branded social images for Posts via Google's Nano
// Banana Gemini image models (CON-105). It renders the reference-first prompt
// scaffold, assembles the multimodal request (brand refs first, subject last),
// drives generation through the Genkit googlegenai plugin, and extracts the
// image defensively from the multi-part response.
//
// v1 runs on the Gemini Developer API; the ImageGenBackend config seam leaves
// room to switch to Vertex AI for EU data residency (CON-109) without touching
// this package's request/response contract.
package imagegen

import (
	"fmt"
	"sort"
	"strings"
)

// Output size is set authoritatively via the API generation config (§7); it
// must not be left to the model to infer, or a mixed-ratio reference set lets
// the last input image silently drive the crop. Every request is validated
// against the selected model's supported ratios/resolutions before dispatch.

// Aspect ratios supported by both image models (§7).
var commonRatios = []string{
	"1:1", "3:2", "2:3", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9",
}

// Extra aspect ratios only Nano Banana 2 (the flash image model) supports (§7).
var flashExtraRatios = []string{"1:4", "4:1", "1:8", "8:1"}

// Resolution tokens as accepted by genai.ImageConfig.ImageSize ("512" is the
// 0.5K draft tier). 1K is the default; 4K is Pro-only; 512 is flash-only (§7).
const (
	Res512 = "512"
	Res1K  = "1K"
	Res2K  = "2K"
	Res4K  = "4K"

	DefaultResolution = Res1K
)

// caps is the set of sizes a model accepts.
type caps struct {
	ratios      map[string]bool
	resolutions map[string]bool
}

func setOf(vals ...[]string) map[string]bool {
	m := map[string]bool{}
	for _, group := range vals {
		for _, v := range group {
			m[v] = true
		}
	}
	return m
}

var (
	// Nano Banana 2 (default, e.g. gemini-3.1-flash-image): the common ratios
	// plus the wide/tall extras, and the 512 draft tier up through 2K. 4K is
	// Pro-only, so it is absent here.
	flashCaps = caps{
		ratios:      setOf(commonRatios, flashExtraRatios),
		resolutions: setOf([]string{Res512, Res1K, Res2K}),
	}
	// Nano Banana Pro (premium, e.g. gemini-3-pro-image): the common ratios and
	// 1K–4K. No 512 draft tier.
	proCaps = caps{
		ratios:      setOf(commonRatios),
		resolutions: setOf([]string{Res1K, Res2K, Res4K}),
	}
	// baseCaps is the conservative fallback for a model id we don't recognise
	// (e.g. a config override or a future `-preview` id): common ratios, 1K–2K.
	baseCaps = caps{
		ratios:      setOf(commonRatios),
		resolutions: setOf([]string{Res1K, Res2K}),
	}
)

// capsFor resolves a model id to its size capabilities. It matches on the
// family substring rather than the exact id so `-preview` suffixes and config
// overrides still land on the right table; anything unrecognised falls back to
// the conservative baseCaps.
func capsFor(modelID string) caps {
	switch {
	case strings.Contains(modelID, "pro-image"):
		return proCaps
	case strings.Contains(modelID, "flash-image"):
		return flashCaps
	default:
		return baseCaps
	}
}

// ValidateSize checks a requested aspect ratio + resolution against the model's
// supported set and returns a clear, caller-facing reason when unsupported (§7).
// A nil return means the pair is safe to dispatch.
func ValidateSize(modelID, aspectRatio, resolution string) error {
	c := capsFor(modelID)
	if !c.ratios[aspectRatio] {
		return fmt.Errorf("aspect ratio %q is not supported by model %q; supported: %s",
			aspectRatio, modelID, strings.Join(sortedKeys(c.ratios), ", "))
	}
	if !c.resolutions[resolution] {
		return fmt.Errorf("resolution %q is not supported by model %q; supported: %s",
			resolution, modelID, strings.Join(sortedKeys(c.resolutions), ", "))
	}
	return nil
}

// SupportsRatio / SupportsResolution expose the per-model predicates for
// callers that resolve defaults (e.g. the Platform→ratio mapping) and want to
// probe before committing to a value.
func SupportsRatio(modelID, aspectRatio string) bool {
	return capsFor(modelID).ratios[aspectRatio]
}

func SupportsResolution(modelID, resolution string) bool {
	return capsFor(modelID).resolutions[resolution]
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
