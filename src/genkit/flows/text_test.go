package flows

import (
	"testing"
)

func TestExtractText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty document",
			input: `[]`,
			want:  "",
		},
		{
			name:  "single paragraph",
			input: `[{"type":"paragraph","content":[{"type":"text","text":"Hello world","styles":{}}],"children":[]}]`,
			want:  "Hello world",
		},
		{
			name: "multiple blocks",
			input: `[
				{"type":"paragraph","content":[{"type":"text","text":"First","styles":{}}],"children":[]},
				{"type":"paragraph","content":[{"type":"text","text":"Second","styles":{}}],"children":[]}
			]`,
			want: "First\nSecond",
		},
		{
			name:  "heading block",
			input: `[{"type":"heading","props":{"level":1},"content":[{"type":"text","text":"My Heading","styles":{}}],"children":[]}]`,
			want:  "My Heading",
		},
		{
			name:  "link inline extracts link text",
			input: `[{"type":"paragraph","content":[{"type":"text","text":"Visit ","styles":{}},{"type":"link","href":"https://example.com","content":[{"type":"text","text":"example","styles":{}}]}],"children":[]}]`,
			want:  "Visit example",
		},
		{
			name: "nested children blocks",
			input: `[{"type":"bulletListItem","content":[{"type":"text","text":"Parent","styles":{}}],"children":[
				{"type":"bulletListItem","content":[{"type":"text","text":"Child","styles":{}}],"children":[]}
			]}]`,
			want: "Parent\nChild",
		},
		{
			name: "mixed block types",
			input: `[
				{"type":"heading","props":{"level":2},"content":[{"type":"text","text":"Section","styles":{}}],"children":[]},
				{"type":"paragraph","content":[{"type":"text","text":"Body text.","styles":{}}],"children":[]}
			]`,
			want: "Section\nBody text.",
		},
		{
			name:  "block with no content",
			input: `[{"type":"paragraph","content":[],"children":[]}]`,
			want:  "",
		},
		{
			name:  "unknown inline type is skipped",
			input: `[{"type":"paragraph","content":[{"type":"text","text":"before","styles":{}},{"type":"unknown","data":"x"},{"type":"text","text":"after","styles":{}}],"children":[]}]`,
			want:  "beforeafter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractText(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractText_InvalidJSON(t *testing.T) {
	_, err := ExtractText(`not json`)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}
