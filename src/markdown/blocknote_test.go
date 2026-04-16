package markdown_test

import (
	"encoding/json"
	"testing"

	"github.com/content-control-center/app/src/markdown"
)

func parse(t *testing.T, md string) []map[string]any {
	t.Helper()
	out := markdown.ToBlocks([]byte(md))
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(out), &blocks); err != nil {
		t.Fatalf("ToBlocks returned invalid JSON: %v\n%s", err, out)
	}
	return blocks
}

func TestHeadings(t *testing.T) {
	blocks := parse(t, "# H1\n\n## H2\n\n### H3\n\n#### H4\n")
	if len(blocks) != 4 {
		t.Fatalf("expected 4 blocks, got %d", len(blocks))
	}
	for i, want := range []float64{1, 2, 3, 3} { // H4 clamps to 3
		if blocks[i]["type"] != "heading" {
			t.Errorf("block %d: type = %v, want heading", i, blocks[i]["type"])
		}
		level := blocks[i]["props"].(map[string]any)["level"]
		if level != want {
			t.Errorf("block %d: level = %v, want %v", i, level, want)
		}
	}
}

func TestParagraph(t *testing.T) {
	blocks := parse(t, "just a paragraph")
	if len(blocks) != 1 || blocks[0]["type"] != "paragraph" {
		t.Fatalf("expected 1 paragraph, got %+v", blocks)
	}
	c := blocks[0]["content"].([]any)
	var combined string
	for _, it := range c {
		combined += it.(map[string]any)["text"].(string)
	}
	if combined != "just a paragraph" {
		t.Errorf("combined text = %q", combined)
	}
}

func TestBulletList(t *testing.T) {
	blocks := parse(t, "- one\n- two\n")
	if len(blocks) != 2 {
		t.Fatalf("expected 2 list items, got %d", len(blocks))
	}
	for i, b := range blocks {
		if b["type"] != "bulletListItem" {
			t.Errorf("item %d: type = %v", i, b["type"])
		}
	}
}

func TestNumberedList(t *testing.T) {
	blocks := parse(t, "1. one\n2. two\n")
	if len(blocks) != 2 {
		t.Fatalf("expected 2 list items, got %d", len(blocks))
	}
	for _, b := range blocks {
		if b["type"] != "numberedListItem" {
			t.Errorf("type = %v", b["type"])
		}
	}
}

func TestNestedList(t *testing.T) {
	blocks := parse(t, "- one\n  - nested\n- two\n")
	if len(blocks) != 2 {
		t.Fatalf("expected 2 top-level items, got %d\n%#v", len(blocks), blocks)
	}
	children := blocks[0]["children"].([]any)
	if len(children) != 1 {
		t.Fatalf("expected 1 nested child, got %d", len(children))
	}
	if children[0].(map[string]any)["type"] != "bulletListItem" {
		t.Errorf("nested type = %v", children[0])
	}
}

func TestQuote(t *testing.T) {
	blocks := parse(t, "> hello\n")
	if len(blocks) != 1 || blocks[0]["type"] != "quote" {
		t.Fatalf("expected 1 quote, got %+v", blocks)
	}
}

func TestFencedCodeBlock(t *testing.T) {
	blocks := parse(t, "```go\nfmt.Println(\"hi\")\n```\n")
	if len(blocks) != 1 || blocks[0]["type"] != "codeBlock" {
		t.Fatalf("expected codeBlock, got %+v", blocks)
	}
	if blocks[0]["props"].(map[string]any)["language"] != "go" {
		t.Errorf("language = %v", blocks[0]["props"])
	}
}

func TestTable(t *testing.T) {
	md := "| a | b |\n| - | - |\n| 1 | 2 |\n"
	blocks := parse(t, md)
	if len(blocks) != 1 || blocks[0]["type"] != "table" {
		t.Fatalf("expected table, got %+v", blocks)
	}
	content := blocks[0]["content"].(map[string]any)
	if content["type"] != "tableContent" {
		t.Errorf("content type = %v", content["type"])
	}
	rows := content["rows"].([]any)
	if len(rows) != 2 {
		t.Errorf("rows = %d", len(rows))
	}
}

func TestThematicBreakIgnored(t *testing.T) {
	blocks := parse(t, "before\n\n---\n\nafter\n")
	if len(blocks) != 2 {
		t.Fatalf("expected 2 paragraphs, got %d", len(blocks))
	}
}

func TestInlineStyles(t *testing.T) {
	blocks := parse(t, "**bold** *ital* `code` ~~strike~~\n")
	inline := blocks[0]["content"].([]any)
	// Find styles per token.
	styles := make([]map[string]any, len(inline))
	for i, it := range inline {
		m := it.(map[string]any)
		if s, ok := m["styles"].(map[string]any); ok {
			styles[i] = s
		}
	}
	// bold
	if styles[0]["bold"] != true {
		t.Errorf("expected bold on token 0, got %v", inline[0])
	}
	// italic (third token — after bold and space)
	foundItalic, foundCode, foundStrike := false, false, false
	for _, s := range styles {
		if s["italic"] == true {
			foundItalic = true
		}
		if s["code"] == true {
			foundCode = true
		}
		if s["strike"] == true {
			foundStrike = true
		}
	}
	if !foundItalic || !foundCode || !foundStrike {
		t.Errorf("missing style. italic=%v code=%v strike=%v", foundItalic, foundCode, foundStrike)
	}
}

func TestLink(t *testing.T) {
	blocks := parse(t, "[hello](https://example.com)\n")
	inline := blocks[0]["content"].([]any)
	link := inline[0].(map[string]any)
	if link["type"] != "link" {
		t.Fatalf("expected link, got %+v", link)
	}
	if link["href"] != "https://example.com" {
		t.Errorf("href = %v", link["href"])
	}
}

func TestEachBlockHasID(t *testing.T) {
	blocks := parse(t, "# a\n\np\n\n- l\n")
	for i, b := range blocks {
		id, _ := b["id"].(string)
		if id == "" {
			t.Errorf("block %d missing id: %+v", i, b)
		}
	}
}

func TestEmptyInput(t *testing.T) {
	out := markdown.ToBlocks([]byte(""))
	var blocks []map[string]any
	if err := json.Unmarshal([]byte(out), &blocks); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Empty input: either [] or a single fallback paragraph. Both are fine.
	if len(blocks) > 1 {
		t.Errorf("expected 0 or 1 blocks, got %d", len(blocks))
	}
}

func TestHTMLBlockFallback(t *testing.T) {
	blocks := parse(t, "<div>raw html</div>\n")
	if len(blocks) == 0 {
		t.Fatalf("expected at least 1 paragraph fallback")
	}
	if blocks[0]["type"] != "paragraph" {
		t.Errorf("expected paragraph fallback, got %v", blocks[0]["type"])
	}
}
