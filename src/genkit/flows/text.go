package flows

import (
	"encoding/json"
	"strings"
)

// blocknoteBlock mirrors the minimal BlockNote block structure needed for
// plain-text extraction. Unknown block types are silently skipped.
//
// Content is held as RawMessage because the shape varies: most blocks use an
// inline array, but table blocks use an object of the form
// { "type": "tableContent", "rows": [{ "cells": [[inline...], ...] }, ...] }.
type blocknoteBlock struct {
	Type     string           `json:"type"`
	Content  json.RawMessage  `json:"content"`
	Children []blocknoteBlock `json:"children"`
}

// blocknoteInline covers StyledText ("text") and Link ("link") inline types.
type blocknoteInline struct {
	Type    string            `json:"type"`
	Text    string            `json:"text"`    // present on "text" nodes
	Content []blocknoteInline `json:"content"` // present on "link" nodes
}

// blocknoteTableContent is the object stored in a table block's `content`
// field. Cells are arrays of inline nodes.
type blocknoteTableContent struct {
	Type string                `json:"type"`
	Rows []blocknoteTableRow   `json:"rows"`
}

type blocknoteTableRow struct {
	Cells [][]blocknoteInline `json:"cells"`
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
	writeBlockContent(sb, b.Content)
	if sb.Len() > before {
		sb.WriteString("\n\n") // paragraph separator consumed by ChunkText
	}
	for _, child := range b.Children {
		writeBlock(sb, child)
	}
}

// writeBlockContent accepts either an inline array or a tableContent object
// and writes the plain text. Unknown shapes are skipped silently.
func writeBlockContent(sb *strings.Builder, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	trimmed := skipWhitespace(raw)
	if len(trimmed) == 0 {
		return
	}
	switch trimmed[0] {
	case '[':
		var inlines []blocknoteInline
		if err := json.Unmarshal(raw, &inlines); err != nil {
			return
		}
		for _, ic := range inlines {
			writeInline(sb, ic)
		}
	case '{':
		var table blocknoteTableContent
		if err := json.Unmarshal(raw, &table); err != nil {
			return
		}
		for ri, row := range table.Rows {
			for ci, cell := range row.Cells {
				for _, ic := range cell {
					writeInline(sb, ic)
				}
				if ci < len(row.Cells)-1 {
					sb.WriteString("\t")
				}
			}
			if ri < len(table.Rows)-1 {
				sb.WriteString("\n")
			}
		}
	}
}

func skipWhitespace(b []byte) []byte {
	for i, c := range b {
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return b[i:]
		}
	}
	return nil
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
