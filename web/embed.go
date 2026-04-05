package web

import (
	"embed"
	"io/fs"
)

// dist holds the compiled React application.
// Run `npm run build` inside the web/ directory before compiling the Go binary.
//
//go:embed dist
var dist embed.FS

// DistFS returns a sub-filesystem rooted at the dist directory.
func DistFS() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
