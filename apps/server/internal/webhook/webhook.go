package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"harbor/internal/netguard"
)

type Target struct {
	URL    string
	Secret string
	Events string
}

func Dispatch(targets []Target, event string, data any) {
	body, err := json.Marshal(map[string]any{
		"event": event,
		"data":  data,
	})
	if err != nil {
		return
	}
	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         netguard.DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
			ForceAttemptHTTP2:   true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	for _, t := range targets {
		if !wants(t.Events, event) {
			continue
		}
		go post(client, t, event, body)
	}
}

func wants(events, event string) bool {
	if strings.TrimSpace(events) == "" || events == "*" {
		return true
	}
	for _, part := range strings.Split(events, ",") {
		if strings.TrimSpace(part) == event {
			return true
		}
	}
	return false
}

func safeWebhookURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "invalid-url"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func post(client *http.Client, t Target, event string, body []byte) {
	mac := hmac.New(sha256.New, []byte(t.Secret))
	mac.Write(body)
	req, err := http.NewRequest(http.MethodPost, t.URL, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Harbor-Webhook")
	req.Header.Set("X-Harbor-Event", event)
	req.Header.Set("X-Harbor-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("webhook %s: %v", safeWebhookURL(t.URL), err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("webhook %s: status %d", safeWebhookURL(t.URL), resp.StatusCode)
	}
}
