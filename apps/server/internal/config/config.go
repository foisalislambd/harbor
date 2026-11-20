package config

import (
	"flag"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Version is the number in VERSION.md. Release builds set it with -ldflags
// from that same file. A local run reads the file when no flag was passed.
var Version = ""

func init() {
	if Version != "" {
		return
	}
	if v := versionFromFile(); v != "" {
		Version = v
		return
	}
	Version = "dev"
}

var versionLine = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

func versionFromFile() string {
	starts := make([]string, 0, 3)
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		starts = append(starts, filepath.Dir(file))
	}
	seen := map[string]struct{}{}
	for _, start := range starts {
		dir := start
		for range 8 {
			if _, ok := seen[dir]; ok {
				break
			}
			seen[dir] = struct{}{}
			if v := parseVersionFile(filepath.Join(dir, "VERSION.md")); v != "" {
				return v
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return ""
}

func parseVersionFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if versionLine.MatchString(line) {
			return line
		}
	}
	return ""
}

type Config struct {
	Addr         string
	DataDir      string
	PanelDir     string
	MaxUpload    int64
	SessionTTL   time.Duration
	CookieSecure bool
	CORSOrigin   string
	TrustProxy   bool
}

func FromEnv() Config {
	return Config{
		Addr:         env("HARBOR_ADDR", ":8787"),
		DataDir:      env("HARBOR_DATA_DIR", "data"),
		PanelDir:     os.Getenv("HARBOR_PANEL_DIR"),
		MaxUpload:    envInt64("HARBOR_MAX_UPLOAD_BYTES", 8<<30, 1, 1<<40),
		SessionTTL:   time.Duration(envInt64("HARBOR_SESSION_HOURS", 24*14, 1, 24*365)) * time.Hour,
		CookieSecure: os.Getenv("HARBOR_COOKIE_SECURE") == "1",
		CORSOrigin:   strings.TrimSpace(os.Getenv("HARBOR_CORS_ORIGIN")),
		TrustProxy:   os.Getenv("HARBOR_TRUST_PROXY") == "1",
	}
}

// Validate checks values that would otherwise be accepted and then misbehave.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Addr) == "" {
		return errConfig("HARBOR_ADDR is empty")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		return errConfig("HARBOR_DATA_DIR is empty")
	}
	if c.MaxUpload <= 0 {
		return errConfig("HARBOR_MAX_UPLOAD_BYTES must be positive")
	}
	if c.SessionTTL <= 0 {
		return errConfig("HARBOR_SESSION_HOURS must be positive")
	}
	if c.CORSOrigin != "" && !validCORSOrigin(c.CORSOrigin) {
		return errConfig("HARBOR_CORS_ORIGIN must be an origin like https://panel.example.com")
	}
	return nil
}

func errConfig(msg string) error { return &configError{msg} }

type configError struct{ msg string }

func (e *configError) Error() string { return e.msg }

func validCORSOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return u.Path == "" || u.Path == "/"
}

func (c *Config) ParseFlags(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.StringVar(&c.Addr, "addr", c.Addr, "listen address")
	fs.StringVar(&c.DataDir, "data", c.DataDir, "data directory (sqlite + blobs)")
	fs.StringVar(&c.PanelDir, "panel", c.PanelDir, "directory of the built panel (optional)")
	return fs.Parse(args)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt64(key string, fallback, min, max int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < min || n > max {
		return fallback
	}
	return n
}
