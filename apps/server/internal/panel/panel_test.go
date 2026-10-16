package panel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceTreeHasNoBundledPanel(t *testing.T) {
	if _, ok := Bundled(); ok {
		t.Fatal("placeholder index.html should not count as a bundled panel")
	}
	if Usable("") {
		t.Fatal("source tree has no usable panel")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Usable(dir) {
		t.Fatal("a directory with index.html is a usable panel")
	}
}
