// Package pdf provides small PDF helpers — page counting and first-page
// thumbnail rendering — used by the post-attachment pipeline (CON-73/75).
//
// Content-bank PDF ingestion (text extraction + page-aware chunking) moved out
// of the API into the pdf-service microservice (CON-103); the extraction and
// chunking code that used to live here was removed with that change.
package pdf

import (
	"bytes"
	"fmt"

	lpdf "github.com/ledongthuc/pdf"
)

// PageCount returns the number of pages in the PDF without extracting text.
func PageCount(data []byte) (int, error) {
	r, err := lpdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, fmt.Errorf("pdf: open: %w", err)
	}
	return r.NumPage(), nil
}
