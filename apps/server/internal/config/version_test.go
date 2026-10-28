package config

import "testing"

func TestVersionComesFromVersionFile(t *testing.T) {
	if Version != "0.1.2" {
		t.Fatalf("version %q, want the number in VERSION.md", Version)
	}
}
