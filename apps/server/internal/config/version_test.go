package config

import "testing"

func TestVersionComesFromVersionFile(t *testing.T) {
	want := versionFromFile()
	if want == "" {
		t.Fatal("VERSION.md was not found from the test working directory")
	}
	if Version != want {
		t.Fatalf("version %q, want %q from VERSION.md", Version, want)
	}
}
