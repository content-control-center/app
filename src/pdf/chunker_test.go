package pdf_test

import (
	"strings"
	"testing"

	"github.com/ogen-app/ogen/src/genkit/flows"
	"github.com/ogen-app/ogen/src/pdf"
)

func TestChunkPages_SingleShortPage(t *testing.T) {
	pages := []pdf.Page{{Num: 1, Text: "Hello world.\n\nSecond paragraph."}}
	chunks := pdf.ChunkPages(pages)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].PageStart != 1 || chunks[0].PageEnd != 1 {
		t.Errorf("unexpected page range: %+v", chunks[0])
	}
	if !strings.Contains(chunks[0].Text, "Hello world") {
		t.Errorf("text missing expected content: %q", chunks[0].Text)
	}
}

func TestChunkPages_MultiPageBelowThreshold(t *testing.T) {
	pages := []pdf.Page{
		{Num: 1, Text: "Page one content."},
		{Num: 2, Text: "Page two content."},
		{Num: 3, Text: "Page three content."},
	}
	chunks := pdf.ChunkPages(pages)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk (short), got %d", len(chunks))
	}
	if chunks[0].PageStart != 1 || chunks[0].PageEnd != 3 {
		t.Errorf("expected span 1–3, got %d–%d", chunks[0].PageStart, chunks[0].PageEnd)
	}
}

func TestChunkPages_LongMultiPage(t *testing.T) {
	// Build pages so the total exceeds MaxEmbedChars and chunking kicks in.
	// Each para ~1,000 chars, 4 paras per page, 3 pages = ~12,000 chars.
	para := strings.Repeat("x", 1000)
	mkPage := func(n int) pdf.Page {
		return pdf.Page{
			Num:  n,
			Text: para + "\n\n" + para + "\n\n" + para + "\n\n" + para,
		}
	}
	pages := []pdf.Page{mkPage(1), mkPage(2), mkPage(3)}
	chunks := pdf.ChunkPages(pages)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	// Every chunk must stay under MaxEmbedChars.
	for i, c := range chunks {
		if len(c.Text) > flows.MaxEmbedChars {
			t.Errorf("chunk %d exceeds MaxEmbedChars: %d", i, len(c.Text))
		}
	}

	// First chunk starts on page 1; last chunk ends on page 3.
	if chunks[0].PageStart != 1 {
		t.Errorf("first chunk should start at page 1, got %d", chunks[0].PageStart)
	}
	if chunks[len(chunks)-1].PageEnd != 3 {
		t.Errorf("last chunk should end at page 3, got %d", chunks[len(chunks)-1].PageEnd)
	}

	// Page ranges must be monotonic non-decreasing.
	for i := 1; i < len(chunks); i++ {
		if chunks[i].PageStart < chunks[i-1].PageStart {
			t.Errorf("chunk %d page_start regressed: %d < %d",
				i, chunks[i].PageStart, chunks[i-1].PageStart)
		}
	}
}

func TestChunkPages_EmptyInput(t *testing.T) {
	if got := pdf.ChunkPages(nil); got != nil {
		t.Errorf("expected nil for empty input, got %+v", got)
	}
	if got := pdf.ChunkPages([]pdf.Page{{Num: 1, Text: "   "}}); got != nil {
		t.Errorf("expected nil for whitespace-only input, got %+v", got)
	}
}

func TestChunkPages_ParagraphLongerThanTarget(t *testing.T) {
	// Single paragraph longer than ChunkTarget forces word-boundary splits.
	big := strings.Repeat("word ", (flows.ChunkTarget/5)+200)
	pages := []pdf.Page{{Num: 1, Text: big}}
	chunks := pdf.ChunkPages(pages)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks from oversized paragraph, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c.Text) > flows.MaxEmbedChars {
			t.Errorf("chunk %d exceeds MaxEmbedChars: %d", i, len(c.Text))
		}
		if c.PageStart != 1 || c.PageEnd != 1 {
			t.Errorf("chunk %d page range wrong: %+v", i, c)
		}
	}
}
