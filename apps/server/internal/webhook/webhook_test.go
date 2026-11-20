package webhook

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatchDoesNotFollowRedirects(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/hook" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "no")
	}))
	defer srv.Close()

	Dispatch([]Target{{URL: srv.URL + "/hook", Secret: "signing-secret", Events: "backup.created"}}, "backup.created", map[string]string{"id": "b"})
	deadline := time.Now().Add(2 * time.Second)
	for hits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if hits.Load() != 1 {
		t.Fatalf("webhook followed a redirect or never fired, hits=%d", hits.Load())
	}
}
