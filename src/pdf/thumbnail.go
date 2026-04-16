package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// ErrThumbnailerUnavailable is returned when the pdftoppm binary is not on PATH.
var ErrThumbnailerUnavailable = errors.New("pdf: pdftoppm unavailable")

// RenderThumbnail returns PNG-encoded bytes of the first page of data at the
// given DPI. Uses pdftoppm (poppler-utils) via exec — no CGo dependency.
//
// Returns ErrThumbnailerUnavailable if pdftoppm is not installed; callers
// should treat this as non-fatal per AC (thumbnail generation is best-effort).
func RenderThumbnail(ctx context.Context, data []byte, dpi int) ([]byte, error) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return nil, ErrThumbnailerUnavailable
	}
	if dpi <= 0 {
		dpi = 96
	}

	in, err := os.CreateTemp("", "ccc-thumb-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("pdf: thumb temp input: %w", err)
	}
	defer os.Remove(in.Name())
	if _, err := in.Write(data); err != nil {
		in.Close()
		return nil, fmt.Errorf("pdf: thumb write: %w", err)
	}
	in.Close()

	// pdftoppm with -singlefile writes to <outPrefix>.png.
	outDir, err := os.MkdirTemp("", "ccc-thumb-out-*")
	if err != nil {
		return nil, fmt.Errorf("pdf: thumb temp dir: %w", err)
	}
	defer os.RemoveAll(outDir)
	outPrefix := outDir + "/thumb"

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx,
		"pdftoppm",
		"-png",
		"-singlefile",
		"-r", fmt.Sprintf("%d", dpi),
		"-f", "1", "-l", "1",
		in.Name(),
		outPrefix,
	)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdf: pdftoppm: %w (%s)", err, stderr.String())
	}

	png, err := os.ReadFile(outPrefix + ".png")
	if err != nil {
		return nil, fmt.Errorf("pdf: read thumbnail: %w", err)
	}
	return png, nil
}
