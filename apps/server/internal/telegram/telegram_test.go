package telegram

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"harbor/internal/copier"
	"harbor/internal/store"
)

func TestPrepareRejectsMetadataAndBadChat(t *testing.T) {
	_, err := Prepare(store.Destination{Provider: store.ProviderTelegram, AccountID: "nope", Secret: "123456:ABCDEFGHIJ"})
	if err == nil {
		t.Fatal("expected bad chat")
	}
	_, err = Prepare(store.Destination{
		Provider: store.ProviderTelegram, AccountID: "-1001", Secret: "123456:ABCDEFGHIJ",
		Endpoint: "http://169.254.169.254",
	})
	if err == nil || !strings.Contains(err.Error(), "link-local") {
		t.Fatal(err)
	}
}

func TestSendSplitsRetriesDeletesAndDownloads(t *testing.T) {
	var mu sync.Mutex
	docs := map[string][]byte{}
	var captions []string
	var deleted []string
	nextMessage := int64(20)
	uploads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":9,"username":"harbor_backup_bot"}}`))
		case strings.HasSuffix(r.URL.Path, "/getChat"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":-1001,"type":"channel"}}`))
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
		case strings.HasSuffix(r.URL.Path, "/deleteMessage"):
			_ = r.ParseForm()
			mu.Lock()
			deleted = append(deleted, r.Form.Get("message_id"))
			mu.Unlock()
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		case strings.HasSuffix(r.URL.Path, "/sendDocument"):
			mu.Lock()
			uploads++
			attempt := uploads
			mu.Unlock()
			if attempt == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":0}}`))
				return
			}
			_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			reader := multipart.NewReader(r.Body, params["boundary"])
			var file []byte
			var caption string
			for {
				part, err := reader.NextPart()
				if err != nil {
					break
				}
				payload, _ := io.ReadAll(part)
				if part.FormName() == "document" {
					file = payload
				}
				if part.FormName() == "caption" {
					caption = string(payload)
				}
			}
			mu.Lock()
			id := nextMessage
			nextMessage++
			docs["file-"+strconv.FormatInt(id, 10)] = file
			captions = append(captions, caption)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":` + strconv.FormatInt(id, 10) + `,"document":{"file_id":"file-` + strconv.FormatInt(id, 10) + `"}}}`))
		case strings.HasSuffix(r.URL.Path, "/getFile"):
			_ = r.ParseForm()
			id := r.Form.Get("file_id")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_id":"` + id + `","file_path":"documents/` + id + `"}}`))
		case strings.Contains(r.URL.Path, "/file/bot"):
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			mu.Lock()
			payload := docs[id]
			mu.Unlock()
			if payload == nil {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := Open(store.Destination{
		Provider: store.ProviderTelegram, AccountID: "-1001", Secret: "123456:ABCDEFGHIJ", Endpoint: srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.HTTP = srv.Client()
	client.PartSize = 8
	if err := client.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.Username != "harbor_backup_bot" {
		t.Fatal(client.Username)
	}
	body := []byte("abcdefghijklmnopqrst")
	path := filepath.Join(t.TempDir(), "blob")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := client.Put(context.Background(), copier.Upload{
		Path: path, Size: int64(len(body)), Checksum: "abc", Project: "Ledger", Name: "nightly", Kind: "database", CreatedAt: "2026-11-21T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 {
		t.Fatalf("parts %d", len(docs))
	}
	joined := append(append([]byte{}, docs["file-20"]...), docs["file-21"]...)
	joined = append(joined, docs["file-22"]...)
	if string(joined) != string(body) {
		t.Fatalf("joined %q", joined)
	}
	if !strings.Contains(captions[0], "Project: Ledger") || !strings.Contains(captions[0], "Part 1 of 3") || !strings.Contains(captions[2], "SHA-256: abc") {
		t.Fatalf("captions %#v", captions)
	}
	got, err := client.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	read, err := io.ReadAll(got)
	if err != nil || string(read) != string(body) {
		t.Fatalf("download %q %v", read, err)
	}
	if err := client.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if strings.Join(deleted, ",") != "7,20,21,22" {
		t.Fatalf("deleted %v", deleted)
	}
}
