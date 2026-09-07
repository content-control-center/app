package zernio_test

import (
	"testing"

	"github.com/ogen-app/ogen/src/infra/publishers/zernio"
)

func TestMediaType(t *testing.T) {
	cases := map[string]string{
		"image/png":       "image",
		"image/jpeg":      "image",
		"image/gif":       "image",
		"image/webp":      "image",
		"video/mp4":       "video",
		"video/quicktime": "video",
		"application/pdf": "document",
		"text/plain":      "",
		"":                "",
	}
	for mime, want := range cases {
		if got := zernio.MediaType(mime); got != want {
			t.Errorf("MediaType(%q) = %q, want %q", mime, got, want)
		}
	}
}
