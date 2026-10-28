package config

import (
	"flag"
	"os"
	"strconv"
	"time"
)

// Version is the release in VERSION.md. Release builds override it with -ldflags.
var Version = "0.1.2"

type Config struct {
	Addr         string
	DataDir      string
	PanelDir     string
	MaxUpload    int64
	SessionTTL   time.Duration
	CookieSecure bool
	CORSOrigin   string
}

func FromEnv() Config {
	return Config{
		Addr:         env("HARBOR_ADDR", ":8787"),
		DataDir:      env("HARBOR_DATA_DIR", "data"),
		PanelDir:     os.Getenv("HARBOR_PANEL_DIR"),
		MaxUpload:    envInt64("HARBOR_MAX_UPLOAD_BYTES", 8<<30),
		SessionTTL:   time.Duration(envInt64("HARBOR_SESSION_HOURS", 24*14)) * time.Hour,
		CookieSecure: os.Getenv("HARBOR_COOKIE_SECURE") == "1",
		CORSOrigin:   os.Getenv("HARBOR_CORS_ORIGIN"),
	}
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

func envInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}
