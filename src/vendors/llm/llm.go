// Package llm registers Ogen's foundation-model vendors — Anthropic (Claude
// generation) and Gemini (embeddings) — into the vendor registry, and exposes
// a Provider that resolves model references by logical role. Flows depend on a
// role ("generation"/"quality") and this package, not on a hardcoded
// "anthropic/" prefix or the Anthropic SDK config type (CON-86 D8/FR12).
//
// Importing this package registers the vendors via init(), mirroring the River
// job self-registration idiom — adding a model vendor is one file.
package llm

import (
	"github.com/firebase/genkit/go/ai"

	"github.com/ogen-app/ogen/src/vendors"
)

// Vendor name slugs — the `vendor` dimension on usage events, and (for
// Anthropic) the genkit "provider/model" prefix.
const (
	VendorAnthropic = "anthropic"
	VendorGemini    = "gemini"
)

// priceVersion tags the rate set below so historical cost is never recomputed
// from edited prices (CON-86 FR3). Bump on any rate change. Rates verified
// 2026-06-30 against platform.claude.com (Sonnet 4.5 $3/$15, Haiku 4.5 $1/$5;
// cache-read 0.1×, cache-write-5m 1.25×) and ai.google.dev (Gemini Embedding
// $0.15/1M input).
const priceVersion = "2026-06-30"

func init() {
	vendors.Register(vendors.Descriptor{
		Name:      VendorAnthropic,
		Family:    vendors.FamilyModel,
		SecretKey: "anthropic_api_key", // must match secrets.NameAnthropicAPIKey
		Metered:   true,
		Meter:     genkitGenerateMeter{},
		Prices: vendors.PriceTable{
			Version: priceVersion,
			Models: map[string]vendors.Rates{
				"claude-sonnet-4-5-20250929": {
					vendors.KindInput:         3_000_000,
					vendors.KindOutput:        15_000_000,
					vendors.KindCacheRead:     300_000,
					vendors.KindCacheCreation: 3_750_000,
				},
				"claude-haiku-4-5-20251001": {
					vendors.KindInput:         1_000_000,
					vendors.KindOutput:        5_000_000,
					vendors.KindCacheRead:     100_000,
					vendors.KindCacheCreation: 1_250_000,
				},
			},
		},
	})

	vendors.Register(vendors.Descriptor{
		Name:      VendorGemini,
		Family:    vendors.FamilyModel,
		SecretKey: "gemini_api_key", // must match secrets.NameGeminiAPIKey (CON-104)
		Metered:   true,
		Meter:     geminiMeter{},
		Prices: vendors.PriceTable{
			Version: priceVersion,
			Models: map[string]vendors.Rates{
				"gemini-embedding-2": {vendors.KindEmbedInput: 150_000},

				// Image models (Nano Banana, CON-105). Rates are TBD pending the
				// §9 credit-unit work and confirmed provider pricing; left
				// count-only (usage tokens are still recorded, cost_micros=0) so
				// we never snapshot a fabricated price. Bump priceVersion when
				// real rates land.
				"gemini-3.1-flash-image": {}, // Nano Banana 2 (default)
				"gemini-3-pro-image":     {}, // Nano Banana Pro (premium)
			},
		},
	})
}

// genkitGenerateMeter extracts token usage from a genkit *ai.ModelResponse.
// Both Anthropic generation flows use it — the mapping is identical across
// genkit model vendors; only the price table differs per vendor.
//
// genkit's normalized usage exposes cache_read (as CachedContentTokens) but
// NOT cache_creation (the Anthropic plugin drops it), so cache_creation is
// left unset (0) and understates cached-write cost until a raw-response path
// lands (CON-86 FR2, §10).
type genkitGenerateMeter struct{}

func (genkitGenerateMeter) Extract(resp any) (string, vendors.Usage, bool) {
	mr, ok := resp.(*ai.ModelResponse)
	if !ok || mr == nil || mr.Usage == nil {
		return "", nil, false
	}
	u := vendors.Usage{}
	addToken(u, vendors.KindInput, mr.Usage.InputTokens)
	addToken(u, vendors.KindOutput, mr.Usage.OutputTokens)
	addToken(u, vendors.KindCacheRead, mr.Usage.CachedContentTokens)
	addToken(u, vendors.KindReasoning, mr.Usage.ThoughtsTokens)
	if len(u) == 0 {
		return "", nil, false
	}
	return "generate", u, true
}

// EmbedUsage is what an embedding call site hands to the Gemini meter: the
// embed response carries no token count, so the caller computes it (from
// assets_chunks.token_count) and passes it here (CON-86 FR2).
type EmbedUsage struct {
	Tokens int64
}

// geminiMeter handles both call shapes that reach the shared "gemini" vendor
// descriptor. Embedding call sites hand it an EmbedUsage; Nano Banana image
// generation (CON-105) hands it a genkit *ai.ModelResponse. The two are told
// apart by response type and tagged with distinct operation_types ("embed" vs
// "generate_image") for §9 accounting. Text generation never reaches here — it
// runs on the anthropic descriptor's genkitGenerateMeter.
type geminiMeter struct{}

func (geminiMeter) Extract(resp any) (string, vendors.Usage, bool) {
	switch v := resp.(type) {
	case EmbedUsage:
		if v.Tokens <= 0 {
			return "", nil, false
		}
		return "embed", vendors.Usage{vendors.KindEmbedInput: v.Tokens}, true
	case *ai.ModelResponse:
		// Reference-image + prompt input tokens count on input (§9); the
		// generated image is billed as output tokens (larger sizes emit more).
		if v == nil || v.Usage == nil {
			return "", nil, false
		}
		u := vendors.Usage{}
		addToken(u, vendors.KindInput, v.Usage.InputTokens)
		addToken(u, vendors.KindOutput, v.Usage.OutputTokens)
		if len(u) == 0 {
			return "", nil, false
		}
		return "generate_image", u, true
	default:
		return "", nil, false
	}
}

func addToken(u vendors.Usage, k vendors.Kind, n int) {
	if n != 0 {
		u[k] = int64(n)
	}
}
