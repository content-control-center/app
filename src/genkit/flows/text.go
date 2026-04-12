package flows

import (
	"encoding/json"
	"strings"
)

// blocknoteBlock mirrors the minimal BlockNote block structure needed for
// plain-text extraction. Unknown block types are silently skipped.
type blocknoteBlock struct {
	Type     string            `json:"type"`
	Content  []blocknoteInline `json:"content"`
	Children []blocknoteBlock  `json:"children"`
}

// blocknoteInline covers StyledText ("text") and Link ("link") inline types.
type blocknoteInline struct {
	Type    string            `json:"type"`
	Text    string            `json:"text"`    // present on "text" nodes
	Content []blocknoteInline `json:"content"` // present on "link" nodes
}

// ExtractText parses a BlockNote JSON document and returns the plain text
// content of all blocks, with blocks separated by newlines.
func ExtractText(blocknoteJSON string) (string, error) {
	var blocks []blocknoteBlock
	if err := json.Unmarshal([]byte(blocknoteJSON), &blocks); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, b := range blocks {
		writeBlock(&sb, b)
	}
	return strings.TrimSpace(sb.String()), nil
}

func writeBlock(sb *strings.Builder, b blocknoteBlock) {
	before := sb.Len()
	for _, ic := range b.Content {
		writeInline(sb, ic)
	}
	if sb.Len() > before {
		sb.WriteByte('\n')
	}
	for _, child := range b.Children {
		writeBlock(sb, child)
	}
}

func writeInline(sb *strings.Builder, ic blocknoteInline) {
	switch ic.Type {
	case "text":
		sb.WriteString(ic.Text)
	case "link":
		for _, c := range ic.Content {
			writeInline(sb, c)
		}
	}
}
