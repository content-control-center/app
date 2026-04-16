package pdf

import "strings"

// Chunk sizing constants mirror those in src/genkit/flows/chunker.go and must
// stay in sync. Duplicated here to avoid a flows → pdf → flows import cycle.
const (
	maxEmbedChars = 6000
	chunkTarget   = 5500
	chunkOverlap  = 500
)

// PagedChunk is a chunk of text with the page range it spans.
type PagedChunk struct {
	Text      string
	PageStart int
	PageEnd   int
}

// ChunkPages splits the per-page plain text into embedding-sized chunks while
// preserving page boundaries. Sizing rules match the global chunker
// (MaxEmbedChars / ChunkTarget / ChunkOverlap from the flows package).
//
// Rules:
//   - Each input page is split into paragraphs (on "\n\n"). Each paragraph
//     carries its source page number.
//   - Paragraphs are accumulated into chunks up to ChunkTarget chars;
//     when a chunk would exceed the target, it is flushed and the next
//     chunk starts with the last ChunkOverlap chars of the previous chunk
//     (overlap inherits the last paragraph's page number).
//   - A single paragraph longer than ChunkTarget is split at word boundaries
//     but keeps its page number.
//   - page_start / page_end track the first and last source page included.
func ChunkPages(pages []Page) []PagedChunk {
	type tagged struct {
		text string
		page int
	}

	var paras []tagged
	for _, p := range pages {
		for _, pr := range strings.Split(strings.TrimSpace(p.Text), "\n\n") {
			pr = strings.TrimSpace(pr)
			if pr == "" {
				continue
			}
			// If a paragraph itself exceeds ChunkTarget, pre-split at word
			// boundaries so the accumulator never blows past MaxEmbedChars.
			for _, sub := range splitLargeAtWord(pr, chunkTarget) {
				paras = append(paras, tagged{text: sub, page: p.Num})
			}
		}
	}
	if len(paras) == 0 {
		return nil
	}

	// Short document: single chunk spanning all pages that contributed text.
	totalLen := 0
	for _, pt := range paras {
		totalLen += len(pt.text) + 2
	}
	if totalLen <= maxEmbedChars {
		var b strings.Builder
		for i, pt := range paras {
			if i > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(pt.text)
		}
		return []PagedChunk{{
			Text:      b.String(),
			PageStart: paras[0].page,
			PageEnd:   paras[len(paras)-1].page,
		}}
	}

	var (
		out        []PagedChunk
		curBuf     strings.Builder
		curStart   int
		curEnd     int
		curStarted bool
	)

	flush := func() {
		t := strings.TrimSpace(curBuf.String())
		if t == "" {
			return
		}
		out = append(out, PagedChunk{Text: t, PageStart: curStart, PageEnd: curEnd})
	}

	for _, pt := range paras {
		// Would appending this paragraph exceed the target? Flush first.
		if curStarted && curBuf.Len()+len(pt.text)+2 > chunkTarget {
			prev := curBuf.String()
			flush()

			// Start a new chunk with overlap from the tail of the previous.
			curBuf.Reset()
			if len(prev) > chunkOverlap {
				curBuf.WriteString(prev[len(prev)-chunkOverlap:])
				curBuf.WriteString("\n\n")
			}
			// Overlap inherits the previous chunk's ending page, and the new
			// chunk starts there too (first real paragraph may bump curEnd).
			curStart = curEnd
		}

		if !curStarted || curBuf.Len() == 0 {
			curStart = pt.page
			curStarted = true
		}
		if curBuf.Len() > 0 {
			curBuf.WriteString("\n\n")
		}
		curBuf.WriteString(pt.text)
		curEnd = pt.page
	}
	flush()

	if len(out) == 0 {
		// Fallback: one chunk with everything.
		var b strings.Builder
		for i, pt := range paras {
			if i > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(pt.text)
		}
		return []PagedChunk{{
			Text:      b.String(),
			PageStart: paras[0].page,
			PageEnd:   paras[len(paras)-1].page,
		}}
	}
	return out
}

func splitLargeAtWord(para string, target int) []string {
	if len(para) <= target {
		return []string{para}
	}
	var parts []string
	remaining := para
	for len(remaining) > target {
		boundary := target
		for boundary > 0 && remaining[boundary] != ' ' {
			boundary--
		}
		if boundary == 0 {
			boundary = target
		}
		parts = append(parts, strings.TrimSpace(remaining[:boundary]))
		remaining = strings.TrimSpace(remaining[boundary:])
	}
	if len(remaining) > 0 {
		parts = append(parts, remaining)
	}
	return parts
}
