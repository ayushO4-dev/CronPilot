// Package cronpilot embeds the built web frontend so the daemon ships as a
// single self-contained binary. The real assets are written to web/dist by the
// frontend build; a committed .gitkeep keeps this directive compilable before
// the first build.
package cronpilot

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var distFS embed.FS

// WebFS returns the embedded built frontend (contents of web/dist) as a
// filesystem rooted at the SPA's index.html.
func WebFS() (fs.FS, error) {
	return fs.Sub(distFS, "web/dist")
}
