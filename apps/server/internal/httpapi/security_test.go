package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"harbor/internal/config"
	"harbor/internal/store"
)

func TestSecurityHeadersAndJSONLimit(t *testing.T) {
	handler := newTestAPI(t).Handler()
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health %d", health.Code)
	}
	if health.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
	if health.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing frame denial")
	}
	if health.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("api responses should not be cached")
	}
	if !strings.Contains(health.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("missing content security policy")
	}

	payload := []byte(`{"email":"` + strings.Repeat("a", maxJSONBody) + `","password":"correct-horse"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body %d %s", res.Code, res.Body.String())
	}
}

func TestLoginLockout(t *testing.T) {
	handler := newTestAPI(t).Handler()
	setup := postJSON(t, handler, "/api/v1/setup", map[string]string{
		"name": "Ada", "email": "ada@example.com", "password": "correct-horse",
	}, "")
	if setup.Code != http.StatusOK {
		t.Fatalf("setup %d %s", setup.Code, setup.Body.String())
	}
	for i := 0; i < maxLoginFails; i++ {
		res := postJSON(t, handler, "/api/v1/auth/login", map[string]string{
			"email": "ada@example.com", "password": "wrong-password",
		}, "")
		if res.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d %s", i, res.Code, res.Body.String())
		}
	}
	locked := postJSON(t, handler, "/api/v1/auth/login", map[string]string{
		"email": "ada@example.com", "password": "correct-horse",
	}, "")
	if locked.Code != http.StatusTooManyRequests {
		t.Fatalf("expected lockout, got %d %s", locked.Code, locked.Body.String())
	}
}

func TestPasswordChangeRevokesOtherSessions(t *testing.T) {
	handler := newTestAPI(t).Handler()
	setup := postJSON(t, handler, "/api/v1/setup", map[string]string{
		"name": "Ada", "email": "ada@example.com", "password": "correct-horse",
	}, "")
	second := postJSON(t, handler, "/api/v1/auth/login", map[string]string{
		"email": "ada@example.com", "password": "correct-horse",
	}, "")
	if second.Code != http.StatusOK {
		t.Fatalf("login %d %s", second.Code, second.Body.String())
	}
	changed := postJSON(t, handler, "/api/v1/auth/password", map[string]string{
		"current_password": "correct-horse",
		"new_password":     "correct-horse-2",
	}, cookie(setup))
	if changed.Code != http.StatusNoContent {
		t.Fatalf("password %d %s", changed.Code, changed.Body.String())
	}
	old := httptest.NewRecorder()
	oldReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	oldReq.Header.Set("Cookie", cookie(second))
	handler.ServeHTTP(old, oldReq)
	if old.Code != http.StatusUnauthorized {
		t.Fatalf("old session %d %s", old.Code, old.Body.String())
	}
	kept := httptest.NewRecorder()
	keptReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	keptReq.Header.Set("Cookie", cookie(setup))
	handler.ServeHTTP(kept, keptReq)
	if kept.Code != http.StatusOK {
		t.Fatalf("current session %d %s", kept.Code, kept.Body.String())
	}

	tooLong := postJSON(t, handler, "/api/v1/auth/password", map[string]string{
		"current_password": "correct-horse-2",
		"new_password":     strings.Repeat("a", 73),
	}, cookie(setup))
	if tooLong.Code != http.StatusBadRequest {
		t.Fatalf("long password %d %s", tooLong.Code, tooLong.Body.String())
	}
}

func TestUserResponseOmitsPassword(t *testing.T) {
	handler := newTestAPI(t).Handler()
	setup := postJSON(t, handler, "/api/v1/setup", map[string]string{
		"name": "Ada", "email": "ada@example.com", "password": "correct-horse",
	}, "")
	var payload struct {
		User map[string]any `json:"user"`
	}
	if err := json.Unmarshal(setup.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload.User["password"]; ok {
		t.Fatalf("password leaked: %s", setup.Body.String())
	}
}

func newTestAPI(t *testing.T) *API {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return New(config.Config{DataDir: dir, MaxUpload: 1 << 20, SessionTTL: time.Hour}, st)
}
