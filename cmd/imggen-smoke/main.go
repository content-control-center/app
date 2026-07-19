// Command imggen-smoke is a throwaway CON-105 smoke test: it exercises the exact
// Genkit path the image-generation feature will use (googlegenai plugin →
// DefineModel with Media support → genkit.Generate with a
// *genai.GenerateContentConfig carrying ResponseModalities + ImageConfig →
// media-part extraction) against the Gemini **Developer API** and answers the
// two questions code alone cannot:
//
//  1. Does the Developer API actually serve the Nano Banana model IDs?
//  2. Does it accept our §7 aspect-ratio / resolution table?
//
// It needs only a Gemini API key (the same gemini_api_key we store for
// embeddings, CON-104) — no GCP/Vertex provisioning. The EU-region check lives
// with the deferred Vertex work (CON-109).
//
// Run:
//
//	GEMINI_API_KEY=... go run ./cmd/imggen-smoke
//	GEMINI_API_KEY=... go run ./cmd/imggen-smoke \
//	    -models gemini-3.1-flash-image,gemini-3-pro-image \
//	    -sizes 1:1@1K,4:5@1K,9:16@1K,16:9@2K,4:5@512 \
//	    -ref ./brand.png -out tmp/imggen-smoke
//
// This is intentionally not wired into the app; delete once CON-105 lands.
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"google.golang.org/genai"
)

func main() {
	var (
		modelsCSV = flag.String("models", "gemini-3.1-flash-image,gemini-3-pro-image", "comma-separated model IDs to probe")
		sizesCSV  = flag.String("sizes", "1:1@1K,4:5@1K,9:16@1K,16:9@2K", "comma-separated ratio@resolution combos (e.g. 4:5@1K, 4:5@512, 16:9@4K)")
		prompt    = flag.String("prompt", "A minimalist product photo of a ceramic coffee mug on a light wooden table, soft natural daylight, shallow depth of field.", "text prompt")
		refPath   = flag.String("ref", "", "optional reference image file to send as a multimodal part (exercises §6 reference path)")
		outDir    = flag.String("out", "tmp/imggen-smoke", "directory to write returned images")
		timeout   = flag.Duration("timeout", 120*time.Second, "per-call timeout")
	)
	flag.Parse()

	key := firstNonEmpty(os.Getenv("GEMINI_API_KEY"), os.Getenv("GOOGLE_API_KEY"))
	if key == "" {
		fmt.Fprintln(os.Stderr, "error: set GEMINI_API_KEY (or GOOGLE_API_KEY). This is the Developer-API key (same value as the gemini_api_key secret); no Vertex/GCP creds needed.")
		os.Exit(2)
	}

	models := splitClean(*modelsCSV)
	sizes := splitClean(*sizesCSV)
	if len(models) == 0 || len(sizes) == 0 {
		fmt.Fprintln(os.Stderr, "error: -models and -sizes must each have at least one entry")
		os.Exit(2)
	}

	// Optional reference image → base64 data URL media part.
	var refPart *ai.Part
	if *refPath != "" {
		b, err := os.ReadFile(*refPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: read -ref %q: %v\n", *refPath, err)
			os.Exit(2)
		}
		mime := mimeFromExt(*refPath)
		refPart = ai.NewMediaPart(mime, "data:"+mime+";base64,"+base64.StdEncoding.EncodeToString(b))
		fmt.Printf("reference image: %s (%d bytes, %s)\n", *refPath, len(b), mime)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: mkdir %q: %v\n", *outDir, err)
		os.Exit(2)
	}

	ctx := context.Background()
	// Developer-API plugin. Swapping this line for
	// &googlegenai.VertexAI{ProjectID, Location} is the whole CON-109 delta —
	// everything below is backend-agnostic (both DefineModel impls share
	// newModel→generate).
	plugin := &googlegenai.GoogleAI{APIKey: key}
	g := genkit.Init(ctx, genkit.WithPlugins(plugin))

	fmt.Printf("\nProbing %d model(s) × %d size(s) via Gemini Developer API\n", len(models), len(sizes))
	fmt.Println(strings.Repeat("-", 72))

	var results []result
	for _, modelID := range models {
		// Image model IDs are not in the plugin's built-in known-model list, so
		// define them explicitly with Media support — mirrors how
		// src/embedding/embedding.go calls DefineEmbedder.
		model, err := plugin.DefineModel(g, modelID, &ai.ModelOptions{
			Label:    modelID,
			Supports: &ai.ModelSupports{Media: true, Multiturn: true},
		})
		if err != nil {
			fmt.Printf("\n[%s] DefineModel failed: %v\n", modelID, err)
			for _, s := range sizes {
				results = append(results, result{model: modelID, size: s, reason: "DefineModel failed"})
			}
			continue
		}

		fmt.Printf("\n[%s]\n", modelID)
		for _, s := range sizes {
			ratio, res, ok := parseSize(s)
			if !ok {
				results = append(results, result{model: modelID, size: s, reason: "unparseable size spec"})
				fmt.Printf("  %-12s SKIP  (bad spec, want ratio@resolution)\n", s)
				continue
			}
			r := probe(ctx, g, model, modelID, *prompt, refPart, ratio, res, *timeout, *outDir)
			results = append(results, r)
			if r.ok {
				fmt.Printf("  %-12s PASS  %d bytes -> %s  [in=%d out=%d total=%d]%s\n",
					s, r.bytes, r.file, r.inTok, r.outTok, r.totalTok, commentarySuffix(r.commentary))
			} else {
				fmt.Printf("  %-12s FAIL  %s\n", s, r.reason)
			}
		}
	}

	printSummary(results)

	// Script-friendly exit: non-zero if nothing succeeded.
	anyPass := false
	for _, r := range results {
		if r.ok {
			anyPass = true
			break
		}
	}
	if !anyPass {
		os.Exit(1)
	}
}

type result struct {
	model      string
	size       string
	ok         bool
	bytes      int
	file       string
	inTok      int
	outTok     int
	totalTok   int
	commentary string
	reason     string
}

// probe issues one generation and reports whether an image came back.
func probe(ctx context.Context, g *genkit.Genkit, model ai.Model, modelID, prompt string, ref *ai.Part, ratio, res string, timeout time.Duration, outDir string) result {
	r := result{model: modelID, size: ratio + "@" + res}

	cfg := &genai.GenerateContentConfig{
		// Nano Banana returns commentary text alongside the image; ask for both.
		ResponseModalities: []string{"TEXT", "IMAGE"},
		ImageConfig: &genai.ImageConfig{
			AspectRatio: ratio, // authoritative output size (§7), not inferred
			ImageSize:   res,
		},
		CandidateCount: 1,
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := []ai.GenerateOption{ai.WithModel(model), ai.WithConfig(cfg)}
	if ref != nil {
		// Reference part first, instruction last (brand-first ordering, §6).
		opts = append(opts, ai.WithMessages(ai.NewUserMessage(ref, ai.NewTextPart(prompt))))
	} else {
		opts = append(opts, ai.WithPrompt(prompt))
	}

	resp, err := genkit.Generate(cctx, g, opts...)
	if err != nil {
		r.reason = truncate(err.Error(), 160)
		return r
	}
	if resp.Usage != nil {
		r.inTok, r.outTok = resp.Usage.InputTokens, resp.Usage.OutputTokens
		r.totalTok = r.inTok + r.outTok
	}

	// Defensive multi-part extraction (§A.3): find the first media part; keep any
	// text as commentary for diagnostics.
	var (
		data []byte
		mime string
	)
	if resp.Message != nil {
		for _, p := range resp.Message.Content {
			switch {
			case p.IsMedia():
				if b, m, derr := decodeMedia(p); derr == nil && data == nil {
					data, mime = b, m
				}
			case p.IsText():
				r.commentary = strings.TrimSpace(p.Text)
			}
		}
	}
	if data == nil {
		if r.commentary != "" {
			r.reason = "no image part (text only: " + truncate(r.commentary, 120) + ")"
		} else {
			r.reason = "no image part in response"
		}
		return r
	}

	name := fmt.Sprintf("%s_%s_%s%s", sanitize(modelID), sanitize(ratio), sanitize(res), extFromMime(mime))
	path := filepath.Join(outDir, name)
	if werr := os.WriteFile(path, data, 0o644); werr != nil {
		r.reason = "decode ok but write failed: " + werr.Error()
		return r
	}
	r.ok, r.bytes, r.file = true, len(data), path
	return r
}

// decodeMedia pulls the bytes out of a returned media part. The googlegenai
// plugin encodes output images as a "data:<mime>;base64,<...>" URL in Part.Text
// (see gemini.go translateCandidate → ai.NewMediaPart).
func decodeMedia(p *ai.Part) ([]byte, string, error) {
	s := p.Text
	const pfx = "data:"
	if !strings.HasPrefix(s, pfx) {
		return nil, "", fmt.Errorf("media part is not a data URL")
	}
	comma := strings.IndexByte(s, ',')
	if comma < 0 {
		return nil, "", fmt.Errorf("malformed data URL")
	}
	meta := s[len(pfx):comma] // e.g. "image/png;base64"
	mime := p.ContentType
	if mime == "" {
		mime = strings.SplitN(meta, ";", 2)[0]
	}
	b, err := base64.StdEncoding.DecodeString(s[comma+1:])
	if err != nil {
		return nil, "", fmt.Errorf("base64 decode: %w", err)
	}
	return b, mime, nil
}

func printSummary(results []result) {
	fmt.Println("\n" + strings.Repeat("=", 72))
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat("=", 72))
	var pass, fail int
	for _, r := range results {
		status := "FAIL"
		detail := r.reason
		if r.ok {
			status, detail = "PASS", fmt.Sprintf("%d bytes, %d tok", r.bytes, r.totalTok)
			pass++
		} else {
			fail++
		}
		fmt.Printf("  %-4s  %-26s %-10s  %s\n", status, r.model, r.size, detail)
	}
	fmt.Printf("\n  %d passed, %d failed\n", pass, fail)
	fmt.Println("  Note: PASS confirms model availability + size acceptance + image-out + usage")
	fmt.Println("        on the Developer API. EU-region residency is validated separately (CON-109).")
}

// --- small helpers ---

func parseSize(s string) (ratio, res string, ok bool) {
	parts := strings.SplitN(s, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), true
}

func splitClean(csv string) []string {
	var out []string
	for _, p := range strings.Split(csv, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func commentarySuffix(c string) string {
	if c == "" {
		return ""
	}
	return "  // " + truncate(c, 60)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func sanitize(s string) string {
	return strings.NewReplacer("/", "-", ":", "-", " ", "_").Replace(s)
}

func extFromMime(mime string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".bin"
	}
}

func mimeFromExt(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	default:
		return "image/png"
	}
}
