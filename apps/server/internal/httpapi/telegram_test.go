package httpapi

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"harbor/internal/config"
	"harbor/internal/store"

	_ "modernc.org/sqlite"
)

func TestTelegramQRRoutesAreRegistered(t *testing.T) {
	handler := newTestAPI(t).Handler()
	for _, path := range []string{
		"/api/v1/destinations/dst_missing/telegram/qr",
		"/api/v1/destinations/dst_missing/telegram/qr/poll",
	} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("X-Harbor-Request", "1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("%s is not registered: %d %s", path, rec.Code, rec.Body.String())
		}
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestTelegramDestinationHidesAPIHash(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	handler := New(config.Config{DataDir: dir, MaxUpload: 1 << 20, SessionTTL: time.Hour}, st).Handler()
	setup := postJSON(t, handler, "/api/v1/setup", map[string]string{
		"name": "Ada", "email": "ada@example.com", "password": "correct-horse",
	}, "")
	hash := "0123456789abcdef0123456789abcdef"
	created := postJSON(t, handler, "/api/v1/destinations", map[string]any{
		"name": "Saved", "provider": "telegram", "account_id": "+15551234567",
		"access_key_id": "123456", "secret_access_key": hash, "endpoint": "",
	}, cookie(setup))
	if created.Code != http.StatusCreated {
		t.Fatalf("create %d %s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), hash) || strings.Contains(created.Body.String(), "session") {
		t.Fatalf("create returned a secret: %s", created.Body.String())
	}
	if !strings.Contains(created.Body.String(), `"signed_in":false`) {
		t.Fatalf("signed in flag: %s", created.Body.String())
	}
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/destinations", nil)
	listReq.Header.Set("Cookie", cookie(setup))
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), hash) {
		t.Fatalf("list %d %s", list.Code, list.Body.String())
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(dir, "harbor.db")), RawQuery: "mode=ro"}).String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var stored string
	if err := db.QueryRow(`SELECT secret_access_key FROM destinations`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stored, "enc:v1:") || strings.Contains(stored, hash) {
		t.Fatalf("secret at rest: %s", stored)
	}
	var id string
	if err := db.QueryRow(`SELECT id FROM destinations`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"name":"Phone","provider":"telegram","account_id":"+15551234567","access_key_id":"123456","endpoint":"@harbor"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/destinations/"+id, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Harbor-Request", "1")
	req.Header.Set("Cookie", cookie(setup))
	renamed := httptest.NewRecorder()
	handler.ServeHTTP(renamed, req)
	if renamed.Code != http.StatusOK || strings.Contains(renamed.Body.String(), hash) || !strings.Contains(renamed.Body.String(), `"name":"Phone"`) {
		t.Fatalf("rename %d %s", renamed.Code, renamed.Body.String())
	}
}
