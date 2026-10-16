package desktop

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindBrowserPrefersPathLookup(t *testing.T) {
	got, ok := findBrowser(env{
		goos: "linux",
		lookup: func(name string) (string, error) {
			if name == "chromium" {
				return "/usr/bin/chromium", nil
			}
			return "", errors.New("missing")
		},
		exists: func(string) bool { return false },
		getenv: func(string) string { return "" },
	})
	if !ok || got != "/usr/bin/chromium" {
		t.Fatalf("got %q %v", got, ok)
	}
}

func TestFindBrowserWindowsInstallPath(t *testing.T) {
	edge := filepath.Join(`C:\Program Files`, "Microsoft", "Edge", "Application", "msedge.exe")
	got, ok := findBrowser(env{
		goos:   "windows",
		lookup: func(string) (string, error) { return "", errors.New("missing") },
		exists: func(path string) bool { return path == edge },
		getenv: func(key string) string {
			if key == "ProgramFiles" {
				return `C:\Program Files`
			}
			return ""
		},
	})
	if !ok || got != edge {
		t.Fatalf("got %q %v", got, ok)
	}
}

func TestAppArgsIsolateTheWindow(t *testing.T) {
	args := strings.Join(appArgs("http://127.0.0.1:8787", filepath.Join("data", "browser-profile")), " ")
	if !strings.Contains(args, "--app=http://127.0.0.1:8787") || !strings.Contains(args, "--user-data-dir=") {
		t.Fatal(args)
	}
}
