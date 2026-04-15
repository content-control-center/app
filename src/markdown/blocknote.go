// Package markdown converts Markdown documents into BlockNote.js Block JSON.
//
// The mapping is defined by CON-46:
//
//	#, ##, ### -> heading (level 1/2/3)
//	paragraph -> paragraph
//	- item -> bulletListItem (nested -> children)
//	1. item -> numberedListItem (nested -> children)
//	> quote -> quote
//	``` code ``` -> codeBlock (props.language from info string)
//	table -> table with tableContent
//	--- -> ignored
//
// Inline mapping: bold, italic, code, strike, link.
// Unsupported nodes fall back to plain paragraphs — no silent drops, no errors.
package markdown

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"

	"github.com/content-control-center/app/src/models"
)

// ToBlocks parses Markdown and returns a serialised BlockNote Block JSON array.
// A parse failure falls back to a single paragraph holding the raw text — the
// call never returns an error.
func ToBlocks(md []byte) string {
	blocks := parseBlocks(md)
	if blocks == nil {
		blocks = []any{paragraphFromText(string(md))}
	}
	out, err := json.Marshal(blocks)
	if err != nil {
		// json.Marshal on []any of our shapes cannot realistically fail.
		// As a last-resort fallback, still emit a single paragraph.
		out, _ = json.Marshal([]any{paragraphFromText(string(md))})
	}
	return string(out)
}

func parseBlocks(md []byte) []any {
	parser := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()
	doc := parser.Parse(text.NewReader(md))
	if doc == nil {
		return nil
	}
	return walkBlocks(doc, md)
}

func walkBlocks(parent ast.Node, src []byte) []any {
	var out []any
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		b := convertBlock(n, src)
		if b == nil {
			continue
		}
		if lc, ok := b.(listContainer); ok {
			out = append(out, lc.items...)
			continue
		}
		out = append(out, b)
	}
	if out == nil {
		out = []any{}
	}
	return out
}

func convertBlock(n ast.Node, src []byte) any {
	switch v := n.(type) {
	case *ast.Heading:
		level := v.Level
		if level > 3 {
			level = 3
		}
		return block("heading", map[string]any{"level": level}, inlineContent(v, src), nil)

	case *ast.Paragraph:
		return block("paragraph", nil, inlineContent(v, src), nil)

	case *ast.TextBlock:
		return block("paragraph", nil, inlineContent(v, src), nil)

	case *ast.List:
		return listBlocks(v, src)

	case *ast.Blockquote:
		return block("quote", nil, blockquoteInline(v, src), nil)

	case *ast.FencedCodeBlock:
		lang := string(v.Language(src))
		return block("codeBlock", map[string]any{"language": lang}, []any{plainText(codeText(v, src))}, nil)

	case *ast.CodeBlock:
		return block("codeBlock", map[string]any{"language": ""}, []any{plainText(codeText(v, src))}, nil)

	case *ast.ThematicBreak:
		return nil // AC: ignored

	case *extast.Table:
		return tableBlock(v, src)

	case *ast.HTMLBlock:
		return paragraphFromText(string(n.Lines().Value(src)))
	}
	return paragraphFromText(string(n.Lines().Value(src)))
}

// listBlocks handles both ordered and unordered lists, flattening into a slice
// result. A list itself is not a BlockNote block — each item becomes a block
// with nested sub-lists folded into `children`.
func listBlocks(list *ast.List, src []byte) any {
	itemType := "bulletListItem"
	if list.IsOrdered() {
		itemType = "numberedListItem"
	}
	items := []any{}
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		items = append(items, listItemBlock(item, itemType, src))
	}
	// Returning a slice here would break the caller's single-node append
	// contract. Wrap multi-item lists by appending directly via a marker:
	// callers use walkBlocks which already iterates — but we are called from
	// convertBlock. To keep it simple, fold top-level list into a sequence via
	// a special container that walkBlocks unwraps.
	if len(items) == 1 {
		return items[0]
	}
	return listContainer{items: items}
}

// listContainer is a private wrapper so walkBlocks can unfold a list into its
// sibling items at the parent level.
type listContainer struct {
	items []any
}

func listItemBlock(item ast.Node, itemType string, src []byte) any {
	var inline []any
	var children []any
	for c := item.FirstChild(); c != nil; c = c.NextSibling() {
		switch c.(type) {
		case *ast.TextBlock, *ast.Paragraph:
			inline = append(inline, inlineFrom(c, src)...)
		case *ast.List:
			sub := convertBlock(c, src)
			switch v := sub.(type) {
			case listContainer:
				children = append(children, v.items...)
			case nil:
				// skip
			default:
				children = append(children, v)
			}
		default:
			if b := convertBlock(c, src); b != nil {
				if lc, ok := b.(listContainer); ok {
					children = append(children, lc.items...)
				} else {
					children = append(children, b)
				}
			}
		}
	}
	if inline == nil {
		inline = []any{}
	}
	return block(itemType, nil, inline, children)
}

// blockquoteInline flattens all inline content from a blockquote's child
// paragraphs into a single inline array (BlockNote quote is a leaf block).
func blockquoteInline(v ast.Node, src []byte) []any {
	var inline []any
	first := true
	for c := v.FirstChild(); c != nil; c = c.NextSibling() {
		part := inlineFrom(c, src)
		if len(part) == 0 {
			continue
		}
		if !first {
			inline = append(inline, plainText("\n"))
		}
		inline = append(inline, part...)
		first = false
	}
	if inline == nil {
		inline = []any{}
	}
	return inline
}

func tableBlock(t *extast.Table, src []byte) any {
	rows := []map[string]any{}
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		row, ok := r.(*extast.TableRow)
		headerRow, isHeader := r.(*extast.TableHeader)
		var rowNode ast.Node
		switch {
		case ok:
			rowNode = row
		case isHeader:
			rowNode = headerRow
		default:
			continue
		}
		cells := [][]any{}
		for c := rowNode.FirstChild(); c != nil; c = c.NextSibling() {
			cell := inlineFrom(c, src)
			if cell == nil {
				cell = []any{}
			}
			cells = append(cells, cell)
		}
		rows = append(rows, map[string]any{"cells": cells})
	}
	content := map[string]any{"type": "tableContent", "rows": rows}
	b := block("table", nil, nil, nil)
	bm := b.(map[string]any)
	bm["content"] = content
	return bm
}

func codeText(n ast.Node, src []byte) string {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		l := lines.At(i)
		b.Write(l.Value(src))
	}
	return strings.TrimRight(b.String(), "\n")
}

func block(kind string, props map[string]any, content []any, children []any) any {
	id, _ := models.NewID()
	if props == nil {
		props = map[string]any{}
	}
	if children == nil {
		children = []any{}
	}
	b := map[string]any{
		"id":       id,
		"type":     kind,
		"props":    props,
		"children": children,
	}
	if content != nil {
		b["content"] = content
	}
	return b
}

// paragraphFromText produces a fallback paragraph block wrapping raw text.
func paragraphFromText(raw string) any {
	raw = strings.TrimRight(raw, "\n")
	inline := []any{}
	if raw != "" {
		inline = append(inline, plainText(raw))
	}
	return block("paragraph", nil, inline, nil)
}

// ---- inline ---------------------------------------------------------------

func inlineContent(parent ast.Node, src []byte) []any {
	out := inlineFrom(parent, src)
	if out == nil {
		out = []any{}
	}
	return out
}

func inlineFrom(parent ast.Node, src []byte) []any {
	var out []any
	styles := map[string]bool{}
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		out = append(out, inlineNode(n, src, styles)...)
	}
	return out
}

func inlineNode(n ast.Node, src []byte, styles map[string]bool) []any {
	switch v := n.(type) {
	case *ast.Text:
		txt := string(v.Segment.Value(src))
		if v.HardLineBreak() || v.SoftLineBreak() {
			txt += "\n"
		}
		if txt == "" {
			return nil
		}
		return []any{styledText(txt, styles)}

	case *ast.String:
		return []any{styledText(string(v.Value), styles)}

	case *ast.CodeSpan:
		sub := map[string]bool{}
		for k, val := range styles {
			sub[k] = val
		}
		sub["code"] = true
		return []any{styledText(childrenText(v, src), sub)}

	case *ast.Emphasis:
		sub := map[string]bool{}
		for k, val := range styles {
			sub[k] = val
		}
		if v.Level >= 2 {
			sub["bold"] = true
		} else {
			sub["italic"] = true
		}
		return collectInlineChildren(v, src, sub)

	case *extast.Strikethrough:
		sub := map[string]bool{}
		for k, val := range styles {
			sub[k] = val
		}
		sub["strike"] = true
		return collectInlineChildren(v, src, sub)

	case *ast.Link:
		return []any{map[string]any{
			"type":    "link",
			"href":    string(v.Destination),
			"content": collectInlineChildren(v, src, styles),
		}}

	case *ast.AutoLink:
		url := string(v.URL(src))
		return []any{map[string]any{
			"type":    "link",
			"href":    url,
			"content": []any{styledText(url, styles)},
		}}

	case *ast.Image:
		alt := childrenText(v, src)
		return []any{styledText(alt, styles)}

	case *ast.RawHTML:
		return []any{styledText(rawHTMLText(v, src), styles)}
	}
	return nil
}

func collectInlineChildren(parent ast.Node, src []byte, styles map[string]bool) []any {
	var out []any
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		out = append(out, inlineNode(n, src, styles)...)
	}
	return out
}

func childrenText(n ast.Node, src []byte) string {
	var b bytes.Buffer
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Text:
			b.Write(v.Segment.Value(src))
		case *ast.String:
			b.Write(v.Value)
		default:
			b.WriteString(childrenText(c, src))
		}
	}
	return b.String()
}

func rawHTMLText(v *ast.RawHTML, src []byte) string {
	var b bytes.Buffer
	for i := 0; i < v.Segments.Len(); i++ {
		s := v.Segments.At(i)
		b.Write(s.Value(src))
	}
	return b.String()
}

func styledText(text string, styles map[string]bool) any {
	s := map[string]any{}
	for k, v := range styles {
		if v {
			s[k] = true
		}
	}
	return map[string]any{
		"type":   "text",
		"text":   text,
		"styles": s,
	}
}

func plainText(text string) any {
	return map[string]any{
		"type":   "text",
		"text":   text,
		"styles": map[string]any{},
	}
}
