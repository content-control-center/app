package postclone

import (
	"testing"

	"github.com/ogen-app/ogen/src/models"
)

func TestDefaultPostType(t *testing.T) {
	tests := []struct {
		name  string
		types models.PostTypeMap
		want  string
	}{
		{"prefers text-post", models.PostTypeMap{"video": "V", "text-post": "T", "image-post": "I"}, "text-post"},
		{"first slug when no text-post", models.PostTypeMap{"video": "V", "image-post": "I"}, "image-post"},
		{"empty map", models.PostTypeMap{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultPostType(tt.types); got != tt.want {
				t.Fatalf("defaultPostType() = %q, want %q", got, tt.want)
			}
		})
	}
}
