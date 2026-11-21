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

func TestTelegramDestinationHidesToken(t *testing.T) {
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
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":9,"username":"harbor_backup_bot"}}`))
		case strings.HasSuffix(r.URL.Path, "/getChat"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":-1001,"type":"channel"}}`))
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
		case strings.HasSuffix(r.URL.Path, "/deleteMessage"):
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer bot.Close()
	token := "123456:ABCDEFGHIJ"
	created := postJSON(t, handler, "/api/v1/destinations", map[string]any{
		"name": "Alerts", "provider": "telegram", "account_id": "-1001234567890",
		"secret_access_key": token, "region": "4", "endpoint": bot.URL,
	}, cookie(setup))
	if created.Code != http.StatusCreated {
		t.Fatalf("create %d %s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), token) {
		t.Fatalf("create returned the token: %s", created.Body.String())
	}
	if !strings.Contains(created.Body.String(), "harbor_backup_bot") {
		t.Fatalf("username missing: %s", created.Body.String())
	}
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/destinations", nil)
	listReq.Header.Set("Cookie", cookie(setup))
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), token) {
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
	if !strings.HasPrefix(stored, "enc:v1:") || strings.Contains(stored, token) {
		t.Fatalf("secret at rest: %s", stored)
	}
	blocked := postJSON(t, handler, "/api/v1/destinations", map[string]any{
		"name": "Bad", "provider": "telegram", "account_id": "-1001234567890",
		"secret_access_key": token, "endpoint": "http://169.254.169.254",
	}, cookie(setup))
	if blocked.Code != http.StatusBadRequest {
		t.Fatalf("metadata %d %s", blocked.Code, blocked.Body.String())
	}
}
