package desktop

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserURL(t *testing.T) {
	cases := map[string]string{
		":8787":          "http://127.0.0.1:8787",
		"0.0.0.0:8787":   "http://127.0.0.1:8787",
		"[::]:8787":      "http://127.0.0.1:8787",
		"127.0.0.1:8787": "http://127.0.0.1:8787",
		"localhost:9000": "http://localhost:9000",
	}
	for addr, want := range cases {
		if got := BrowserURL(addr); got != want {
			t.Fatalf("BrowserURL(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestHealthyRequiresHarborPayload(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"version":"0.1.0"}`))
	}))
	defer ok.Close()
	if !Healthy(ok.URL) {
		t.Fatal("expected a Harbor health payload to count as healthy")
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer other.Close()
	if Healthy(other.URL) {
		t.Fatal("a non-Harbor 200 response must not count as healthy")
	}
}
