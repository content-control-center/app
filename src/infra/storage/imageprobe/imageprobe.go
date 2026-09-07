// Package imageprobe inspects image uploads to extract structural
// metadata (MIME, dimensions, animated-GIF flag, SHA-256). It validates
// by decoding image headers — never trusting the client's declared
// Content-Type — so a `.jpg` named `payload.exe` is rejected before
// upload (CON-73 §2.2).
package imageprobe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"io"
	"net/http"

	// Register decoders so image.DecodeConfig can resolve JPEG/PNG/GIF
	// formats. WebP is added via the chai2010/webp side-effect import.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// AllowedMIMEs is the set of MIME types accepted for post attachments.
var AllowedMIMEs = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// Result carries everything the caller needs after probing — the
// decoded MIME, image dimensions, animated-GIF flag, and the SHA-256
// of the bytes that were probed.
type Result struct {
	MIME       string
	Extension  string
	Width      int
	Height     int
	IsAnimated bool
	SHA256     string
	Size       int64
}

// ErrUnsupportedMIME is returned when the decoded content type is not
// one of AllowedMIMEs.
var ErrUnsupportedMIME = errors.New("imageprobe: unsupported media type")

// Probe reads r in full, computes SHA-256, sniffs the MIME from the
// magic bytes, and decodes structural metadata. r must be seekable;
// pass a *bytes.Reader or buffer the upload first. limit caps the
// number of bytes read; reading more returns an error so handlers can
// reject oversized uploads without exhausting memory.
//
// On success, the caller can re-read r from the start (e.g. by calling
// Seek(0, 0) on a *bytes.Reader, or by re-wrapping the buffered bytes)
// to stream the same content to S3.
func Probe(r io.Reader, limit int64) (*Result, []byte, error) {
	// Buffer up to limit+1 so we can detect truncation.
	buf := &bytes.Buffer{}
	n, err := io.Copy(buf, io.LimitReader(r, limit+1))
	if err != nil {
		return nil, nil, fmt.Errorf("imageprobe: read: %w", err)
	}
	if n > limit {
		return nil, nil, fmt.Errorf("imageprobe: file exceeds limit of %d bytes", limit)
	}
	data := buf.Bytes()
	if len(data) == 0 {
		return nil, nil, errors.New("imageprobe: empty file")
	}

	mime := http.DetectContentType(data[:min(len(data), 512)])
	ext, ok := AllowedMIMEs[mime]
	if !ok {
		return nil, nil, fmt.Errorf("%w: %s", ErrUnsupportedMIME, mime)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("imageprobe: decode header: %w", err)
	}

	res := &Result{
		MIME:      mime,
		Extension: ext,
		Width:     cfg.Width,
		Height:    cfg.Height,
		Size:      n,
	}

	if mime == "image/gif" {
		// gif.DecodeAll returns frame count via len(g.Image); >1 means animated.
		g, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return nil, nil, fmt.Errorf("imageprobe: decode gif: %w", err)
		}
		res.IsAnimated = len(g.Image) > 1
	}

	sum := sha256.Sum256(data)
	res.SHA256 = hex.EncodeToString(sum[:])

	return res, data, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
