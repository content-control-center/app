package imagegen

import "github.com/ogen-app/ogen/src/usage"

// ImagineRequest is the input to the imagine flow: a prompt-driving description
// plus optional brand-kit and subject reference images, and the authoritative
// output size (§7). Reference bytes arrive already loaded — the River worker
// fetches them from S3 — so this package does no storage I/O.
type ImagineRequest struct {
	Platform    string           `json:"platform"`
	SubjectDesc string           `json:"subjectDesc"`
	Instruction string           `json:"instruction"`
	BrandRefs   []ReferenceImage `json:"-"`           // style role; ordered first (§6)
	Subject     *ReferenceImage  `json:"-"`           // content role; placed last (§6)
	AspectRatio string           `json:"aspectRatio"` // authoritative ratio (§7); required
	Resolution  string           `json:"resolution"`  // authoritative resolution (§7); "" -> 1K
	Premium     bool             `json:"premium"`     // use the Pro model tier
}

// ReferenceImage is a decoded reference image passed as a multimodal part.
type ReferenceImage struct {
	MIMEType string
	Data     []byte
}

// ImagineResponse is a completed generation. AspectRatio/Resolution/Model are
// echoed back for the caller to persist on the Asset for reproducibility (§7).
// The output carries SynthID + C2PA content credentials inherently (§10).
type ImagineResponse struct {
	Image       []byte `json:"-"`
	MIMEType    string `json:"mimeType"`
	Model       string `json:"model"`
	AspectRatio string `json:"aspectRatio"`
	Resolution  string `json:"resolution"`
}

// ImagineFlowConfig holds the flow's static dependencies. Model is the
// reloadable Gemini image-model provider (built in server wiring, analogous to
// the embedder threaded into the embed flow). Recorder/Checker are the CON-86
// usage layer and are nil-safe (nil when analytics is disabled).
type ImagineFlowConfig struct {
	Model    *Model
	Recorder *usage.Recorder
	Checker  *usage.Checker
	Feature  string // usage-event feature slug; "" -> "post_image"
}

// ── Errors (mapped to HTTP codes by the caller) ─────────────────────────────

// ValidationError → HTTP 400. Bad input or an unsupported/invalid output size.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// AIError → HTTP 502. A model-call failure or an empty (no-image) response.
type AIError struct{ Msg string }

func (e *AIError) Error() string { return e.Msg }
