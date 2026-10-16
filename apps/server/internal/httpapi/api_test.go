package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"harbor/internal/config"
	"harbor/internal/store"
)

func TestSetupLoginAndPush(t *testing.T) {
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
	if setup.Code != http.StatusOK {
		t.Fatalf("setup %d %s", setup.Code, setup.Body.String())
	}
	again := postJSON(t, handler, "/api/v1/setup", map[string]string{
		"name": "Other", "email": "other@example.com", "password": "correct-horse",
	}, "")
	if again.Code != http.StatusConflict {
		t.Fatalf("second setup %d %s", again.Code, again.Body.String())
	}

	project := postJSON(t, handler, "/api/v1/projects", map[string]any{
		"name": "Ledger", "description": "nightly dumps", "retention_count": 5, "retention_days": 14, "quota_bytes": 0,
	}, cookie(setup))
	if project.Code != http.StatusCreated {
		t.Fatalf("project %d %s", project.Code, project.Body.String())
	}
	var created store.Project
	if err := json.Unmarshal(project.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	keyRes := postJSON(t, handler, "/api/v1/projects/"+created.ID+"/keys", map[string]string{"name": "ci"}, cookie(setup))
	if keyRes.Code != http.StatusCreated {
		t.Fatalf("key %d %s", keyRes.Code, keyRes.Body.String())
	}
	var key store.APIKey
	if err := json.Unmarshal(keyRes.Body.Bytes(), &key); err != nil {
		t.Fatal(err)
	}
	if key.Token == "" {
		t.Fatal("expected one-time token")
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("name", "nightly")
	_ = w.WriteField("kind", "database")
	_ = w.WriteField("source", "ledger")
	part, err := w.CreateFormFile("file", "dump.sql.gz")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("select 1;"))
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/push", &body)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("push %d %s", res.Code, res.Body.String())
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil)
	listReq.Header.Set("Authorization", "Bearer "+key.Token)
	handler.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK {
		t.Fatalf("list %d %s", list.Code, list.Body.String())
	}
	if !bytes.Contains(list.Body.Bytes(), []byte("nightly")) {
		t.Fatalf("missing backup: %s", list.Body.String())
	}
}

func postJSON(t *testing.T, h http.Handler, path string, payload any, cookieHeader string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func cookie(res *httptest.ResponseRecorder) string {
	cookies := res.Result().Cookies()
	if len(cookies) == 0 {
		return ""
	}
	return cookies[0].Name + "=" + cookies[0].Value
}

func TestPausedProjectRejectsPush(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	handler := New(config.Config{DataDir: dir, MaxUpload: 1024, SessionTTL: time.Hour}, st).Handler()
	setup := postJSON(t, handler, "/api/v1/setup", map[string]string{
		"name": "Ada", "email": "ada@example.com", "password": "correct-horse",
	}, "")
	project := postJSON(t, handler, "/api/v1/projects", map[string]any{
		"name": "Paused", "description": "", "retention_count": 1, "retention_days": 1, "quota_bytes": 0,
	}, cookie(setup))
	var created store.Project
	_ = json.Unmarshal(project.Body.Bytes(), &created)
	raw, _ := json.Marshal(map[string]any{
		"name": "Paused", "description": "", "status": "paused", "retention_count": 1, "retention_days": 1, "quota_bytes": 0,
	})
	pauseReq := httptest.NewRequest(http.MethodPatch, "/api/v1/projects/"+created.ID, bytes.NewReader(raw))
	pauseReq.Header.Set("Content-Type", "application/json")
	pauseReq.Header.Set("Cookie", cookie(setup))
	paused := httptest.NewRecorder()
	handler.ServeHTTP(paused, pauseReq)
	if paused.Code != http.StatusOK {
		t.Fatalf("patch %d %s", paused.Code, paused.Body.String())
	}
	keyRes := postJSON(t, handler, "/api/v1/projects/"+created.ID+"/keys", map[string]string{"name": "ci"}, cookie(setup))
	var key store.APIKey
	_ = json.Unmarshal(keyRes.Body.Bytes(), &key)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", "a.bin")
	_, _ = io.WriteString(part, "abc")
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/push", &body)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected conflict, got %d %s", res.Code, res.Body.String())
	}
}

func TestReadOnlyKeyCannotPush(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	handler := New(config.Config{DataDir: dir, MaxUpload: 1024, SessionTTL: time.Hour}, st).Handler()
	setup := postJSON(t, handler, "/api/v1/setup", map[string]string{
		"name": "Ada", "email": "ada@example.com", "password": "correct-horse",
	}, "")
	project := postJSON(t, handler, "/api/v1/projects", map[string]any{
		"name": "Read", "description": "", "retention_count": 5, "retention_days": 0, "quota_bytes": 0,
	}, cookie(setup))
	var created store.Project
	if err := json.Unmarshal(project.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	keyRes := postJSON(t, handler, "/api/v1/projects/"+created.ID+"/keys", map[string]string{
		"name": "reader", "scopes": "backup:read",
	}, cookie(setup))
	var key store.APIKey
	if err := json.Unmarshal(keyRes.Body.Bytes(), &key); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", "a.bin")
	_, _ = io.WriteString(part, "abc")
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/push", &body)
	req.Header.Set("Authorization", "Bearer "+key.Token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d %s", res.Code, res.Body.String())
	}
}
