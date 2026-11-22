package config

import (
	"testing"
	"time"
)

func TestFromEnvClampsInvalidNumbers(t *testing.T) {
	t.Setenv("HARBOR_MAX_UPLOAD_BYTES", "-5")
	t.Setenv("HARBOR_SESSION_HOURS", "0")
	t.Setenv("HARBOR_CORS_ORIGIN", "https://panel.example.com")
	t.Setenv("HARBOR_TRUST_PROXY", "1")
	t.Setenv("HARBOR_COOKIE_SECURE", "1")

	cfg := FromEnv()
	if cfg.MaxUpload != 8<<30 {
		t.Fatalf("max upload %d", cfg.MaxUpload)
	}
	if cfg.SessionTTL != 14*24*time.Hour {
		t.Fatalf("session ttl %s", cfg.SessionTTL)
	}
	if !cfg.TrustProxy || !cfg.CookieSecure {
		t.Fatalf("flags were not read: %+v", cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.CORSOrigin = "https://panel.example.com/path"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a path in the CORS origin to be rejected")
	}
}

func TestPublicURL(t *testing.T) {
	t.Setenv("HARBOR_PUBLIC_URL", "https://harbor.example.com/")
	cfg := FromEnv()
	if cfg.PublicURL != "https://harbor.example.com" {
		t.Fatal(cfg.PublicURL)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	cfg.PublicURL = "https://harbor.example.com/api"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected a path in the public URL to be rejected")
	}
}
