package panel

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:dist
var dist embed.FS

// Bundled is the built panel shipped inside a release binary.
// A source checkout keeps a placeholder page here, which is not a UI.
func Bundled() (fs.FS, bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	body, err := fs.ReadFile(sub, "index.html")
	if err != nil || bytes.Contains(body, []byte("harbor-panel-placeholder")) {
		return nil, false
	}
	return sub, true
}

// Usable reports whether dir contains a built panel, or the binary has one embedded.
func Usable(dir string) bool {
	if dir != "" {
		info, err := os.Stat(filepath.Join(dir, "index.html"))
		if err == nil && !info.IsDir() {
			return true
		}
	}
	_, ok := Bundled()
	return ok
}
