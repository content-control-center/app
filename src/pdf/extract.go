// Package pdf handles PDF text extraction, page-aware chunking, and
// thumbnail rendering for PDF-backed Assets.
package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	lpdf "github.com/ledongthuc/pdf"
)

// Page holds the extracted plain text for a single PDF page.
type Page struct {
	Num  int
	Text string
}

// fallbackTextThreshold is the minimum character count (over all pages) below
// which ExtractText falls back to pdftotext for multi-page PDFs.
const fallbackTextThreshold = 100

// ErrEmptyPDF is returned when extraction produces zero characters even after
// the fallback path.
var ErrEmptyPDF = errors.New("pdf: no text extracted")

// ExtractText returns per-page plain text for the PDF in data.
//
// Pipeline:
//  1. Try ledongthuc/pdf (pure Go, no CGo).
//  2. If multi-page and total chars < fallbackTextThreshold, try pdftotext.
//
// When the fallback path is used, per-page granularity is lost — all text
// collapses into page 1. Callers that require page attribution should prefer
// the primary path.
func ExtractText(ctx context.Context, data []byte) ([]Page, error) {
	pages, err := extractWithLedongthuc(data)
	if err == nil && sufficientText(pages) {
		return pages, nil
	}

	// Fallback: pdftotext via exec. Requires poppler-utils on PATH.
	fbPages, fbErr := extractWithPdftotext(ctx, data)
	if fbErr == nil && len(fbPages) > 0 {
		return fbPages, nil
	}

	// Return primary result if non-empty; otherwise surface an error.
	if len(pages) > 0 {
		return pages, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, ErrEmptyPDF
}

func sufficientText(pages []Page) bool {
	if len(pages) <= 1 {
		return totalChars(pages) > 0
	}
	return totalChars(pages) >= fallbackTextThreshold
}

func totalChars(pages []Page) int {
	n := 0
	for _, p := range pages {
		n += len(p.Text)
	}
	return n
}

// PageCount returns the number of pages in the PDF without extracting text.
func PageCount(data []byte) (int, error) {
	r, err := lpdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("pdf: open: %w", err)
	}
	return r.NumPage(), nil
}

func extractWithLedongthuc(data []byte) ([]Page, error) {
	r, err := lpdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("pdf: open: %w", err)
	}

	total := r.NumPage()
	pages := make([]Page, 0, total)
	for i := 1; i <= total; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			// Per-page failures are non-fatal: keep going, record empty text.
			text = ""
		}
		pages = append(pages, Page{Num: i, Text: strings.TrimSpace(text)})
	}
	return pages, nil
}

func extractWithPdftotext(ctx context.Context, data []byte) ([]Page, error) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return nil, fmt.Errorf("pdf: pdftotext unavailable: %w", err)
	}

	f, err := os.CreateTemp("", "ccc-pdf-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("pdf: temp file: %w", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return nil, fmt.Errorf("pdf: write temp: %w", err)
	}
	f.Close()

	var stdout bytes.Buffer
	cmd := exec.CommandContext(ctx, "pdftotext", "-layout", "-enc", "UTF-8", f.Name(), "-")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdf: pdftotext: %w", err)
	}

	text := strings.TrimSpace(stdout.String())
	if text == "" {
		return nil, ErrEmptyPDF
	}
	// pdftotext emits a form-feed (\f) between pages. Split on that so we keep
	// at least coarse page granularity on the fallback path.
	parts := strings.Split(text, "\f")
	pages := make([]Page, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		pages = append(pages, Page{Num: i + 1, Text: p})
	}
	return pages, nil
}

