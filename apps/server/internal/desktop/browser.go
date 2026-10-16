package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

var errNotReady = errors.New("harbor did not start")

// Launch opens the panel and blocks until the user is done with this process.
// ownsServer is false when Harbor is already running in another process: this
// process must not stay alive after the extra window is gone, and it must not
// be the thing that keeps the server alive.
func Launch(url, profile string, ownsServer bool) error {
	if err := os.MkdirAll(profile, 0o755); err != nil {
		return err
	}
	if openNative(url, filepath.Join(profile, "webview")) {
		return nil
	}
	browserProfile := filepath.Join(profile, "browser")
	if err := os.MkdirAll(browserProfile, 0o755); err != nil {
		return err
	}
	if bin, ok := findBrowser(hostEnv()); ok {
		return runAppBrowser(bin, url, browserProfile, ownsServer)
	}
	if err := openSystemBrowser(url); err != nil {
		return err
	}
	if !ownsServer {
		return nil
	}
	return StayOpen(url)
}

// StayOpen keeps the process alive when there is no desktop window whose
// lifetime matches this process. Closing that wait stops a server this process owns.
func StayOpen(url string) error {
	if runtime.GOOS == "windows" {
		Info("Harbor", "Harbor is running at "+url+".\n\nClick OK to stop.")
		return nil
	}
	fmt.Fprintf(os.Stderr, "Harbor is running at %s\nPress Ctrl+C to stop.\n", url)
	WaitInterrupt()
	return nil
}

func runAppBrowser(bin, url, profile string, ownsServer bool) error {
	cmd := exec.Command(bin, appArgs(url, profile)...)
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err != nil || !ownsServer {
			return err
		}
		// Edge and Chrome exit immediately when they hand the window to a
		// process that is already running. The page is open; keep serving.
		return StayOpen(url)
	case <-time.After(2 * time.Second):
		return <-exited
	}
}

func openSystemBrowser(url string) error {
	name, args := fallbackCommand(runtime.GOOS, url)
	if name == "" {
		return errors.New("no desktop browser found; install Microsoft Edge, Chrome, or Chromium")
	}
	cmd := exec.Command(name, args...)
	return cmd.Start()
}

func appArgs(url, profile string) []string {
	return []string{
		"--app=" + url,
		"--user-data-dir=" + profile,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--window-size=1280,800",
	}
}

type env struct {
	goos   string
	lookup func(string) (string, error)
	exists func(string) bool
	getenv func(string) string
}

func hostEnv() env {
	return env{
		goos:   runtime.GOOS,
		lookup: exec.LookPath,
		exists: fileExists,
		getenv: os.Getenv,
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func findBrowser(e env) (string, bool) {
	var names []string
	var paths []string
	switch e.goos {
	case "windows":
		names = []string{"msedge.exe", "chrome.exe", "brave.exe", "msedge", "chrome", "brave"}
		for _, root := range []string{
			e.getenv("ProgramFiles"),
			e.getenv("ProgramFiles(x86)"),
			e.getenv("LocalAppData"),
		} {
			if root == "" {
				continue
			}
			paths = append(paths,
				filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(root, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			)
		}
	case "darwin":
		paths = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
		names = []string{"google-chrome", "chromium", "microsoft-edge"}
	default:
		names = []string{
			"google-chrome",
			"google-chrome-stable",
			"chromium",
			"chromium-browser",
			"microsoft-edge",
			"microsoft-edge-stable",
			"brave-browser",
		}
	}
	for _, name := range names {
		if path, err := e.lookup(name); err == nil && path != "" {
			return path, true
		}
	}
	for _, path := range paths {
		if e.exists(path) {
			return path, true
		}
	}
	return "", false
}

func fallbackCommand(goos, url string) (string, []string) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		return "open", []string{url}
	default:
		return "xdg-open", []string{url}
	}
}
