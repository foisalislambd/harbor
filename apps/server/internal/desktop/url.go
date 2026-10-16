package desktop

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// BrowserURL is the loopback URL a desktop window should open for a listen address.
func BrowserURL(addr string) string {
	host := addr
	switch {
	case strings.HasPrefix(host, ":"):
		host = "127.0.0.1" + host
	case strings.HasPrefix(host, "0.0.0.0:"):
		host = "127.0.0.1:" + strings.TrimPrefix(host, "0.0.0.0:")
	case strings.HasPrefix(host, "[::]:"):
		host = "127.0.0.1:" + strings.TrimPrefix(host, "[::]:")
	}
	return "http://" + host
}

// Healthy reports whether a Harbor API is already answering at baseURL.
func Healthy(baseURL string) bool {
	client := &http.Client{Timeout: 400 * time.Millisecond}
	resp, err := client.Get(baseURL + "/api/v1/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return false
	}
	return body.OK
}

// WaitReady polls until the local server answers or the timeout passes.
func WaitReady(baseURL string) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if Healthy(baseURL) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errNotReady
}
