package telegram

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"harbor/internal/copier"
	"harbor/internal/store"
)

func TestPrepareAndLimits(t *testing.T) {
	got, err := Prepare(store.Destination{
		Provider: store.ProviderTelegram, AccountID: "+1 (555) 123-4567", AccessKeyID: "123456",
		Secret: "0123456789abcdef0123456789abcdef", Endpoint: "saved messages", Region: "4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.AccountID != "+15551234567" || got.Endpoint != "" || !strings.Contains(got.Secret, "0123456789abcdef0123456789abcdef") {
		t.Fatalf("%+v", got)
	}
	if _, err := Prepare(store.Destination{Provider: store.ProviderTelegram, AccountID: "nope", AccessKeyID: "123456", Secret: "0123456789abcdef0123456789abcdef"}); err == nil {
		t.Fatal("expected a phone error")
	}
	if _, err := Prepare(store.Destination{Provider: store.ProviderTelegram, AccountID: "+15551234567", AccessKeyID: "abc", Secret: "0123456789abcdef0123456789abcdef"}); err == nil {
		t.Fatal("expected an api id error")
	}
	limit := DocumentLimit(false)
	if partCount(limit, limit) != 1 || partCount(limit+1, limit) != 2 {
		t.Fatalf("parts %d %d", partCount(limit, limit), partCount(limit+1, limit))
	}
	if DocumentLimit(true) != limit*2 {
		t.Fatalf("premium limit %d", DocumentLimit(true))
	}
	if !strings.Contains(caption("Ledger", "nightly", "database", "2026-11-21T00:00:00Z", "abc", 20, 1, 3), "Part 1 of 3") {
		t.Fatal("caption")
	}
}

func TestLoginUploadAndDelete(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef"
	raw, err := encodeCredentials(credentials{APIHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	client, err := Open(store.Destination{
		Provider: store.ProviderTelegram, AccountID: "+15551234567", AccessKeyID: "123456",
		Secret: raw, Endpoint: "@harborbackups", Region: "4",
	})
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeEngine{passwordNeeded: true, files: map[int][]byte{}}
	client.engine = fake
	client.limit = 8
	delivery, err := client.SendCode(context.Background())
	if err != nil || delivery != "app" || client.cred.PhoneCodeHash == "" {
		t.Fatalf("send code %q %v hash %q", delivery, err, client.cred.PhoneCodeHash)
	}
	if _, err := client.SignIn(context.Background(), "12345"); !errors.Is(err, ErrPasswordNeeded) {
		t.Fatal(err)
	}
	name, err := client.Password(context.Background(), "cloud")
	if err != nil || name != "Ada (@ada)" || !client.cred.Authorized {
		t.Fatalf("password %q %v authorized %v", name, err, client.cred.Authorized)
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
	if len(fake.files) != 3 || !strings.Contains(fake.captions[0], "Project: Ledger") || fake.names[2] != "harbor-nightly-part03-of-03.bin" {
		t.Fatalf("uploads %#v %#v", fake.names, fake.captions)
	}
	joined := append(append([]byte{}, fake.files[1]...), fake.files[2]...)
	joined = append(joined, fake.files[3]...)
	if string(joined) != string(body) {
		t.Fatalf("joined %q", joined)
	}
	got, err := client.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	read, err := io.ReadAll(got)
	got.Close()
	if err != nil || string(read) != string(body) {
		t.Fatalf("download %q %v", read, err)
	}
	if err := client.Delete(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fake.deleted, ",") != "1,2,3" {
		t.Fatalf("deleted %v", fake.deleted)
	}
	if err := client.LogOut(context.Background()); err != nil || client.cred.Authorized || len(client.cred.Session) != 0 {
		t.Fatalf("logout %+v %v", client.cred, err)
	}
	signed, label := PublicStatus(client.Secret())
	if signed || label != "" || !strings.Contains(client.Secret(), hash) {
		t.Fatalf("status %v %q secret %s", signed, label, client.Secret())
	}
}

type fakeEngine struct {
	passwordNeeded bool
	files          map[int][]byte
	names          []string
	captions       []string
	deleted        []string
	next           int
}

func (f *fakeEngine) SendCode(_ context.Context, _ int, cred *credentials, _ string) (string, error) {
	cred.PhoneCodeHash = "code-hash"
	cred.Session = []byte("key")
	return "app", nil
}

func (f *fakeEngine) SignIn(_ context.Context, _ int, cred *credentials, _, _ string) (string, error) {
	cred.Session = []byte("pending")
	if f.passwordNeeded {
		return "", ErrPasswordNeeded
	}
	return "Ada (@ada)", nil
}

func (f *fakeEngine) Password(_ context.Context, _ int, cred *credentials, _ string) (string, error) {
	cred.Session = []byte("ready")
	return "Ada (@ada)", nil
}

func (f *fakeEngine) LogOut(context.Context, int, *credentials) error { return nil }

func (f *fakeEngine) Self(context.Context, int, *credentials) (string, bool, error) {
	return "Ada (@ada)", false, nil
}

func (f *fakeEngine) Resolve(_ context.Context, _ int, _ *credentials, target string) (storedPeer, error) {
	return storedPeer{Kind: "channel", ID: 9, Access: 8, Target: target}, nil
}

func (f *fakeEngine) Upload(_ context.Context, _ int, _ *credentials, _ storedPeer, _ int, file *os.File, offset, n int64, filename, caption string) (int, error) {
	buf := make([]byte, n)
	if _, err := file.ReadAt(buf, offset); err != nil {
		return 0, err
	}
	f.next++
	f.files[f.next] = buf
	f.names = append(f.names, filename)
	f.captions = append(f.captions, caption)
	return f.next, nil
}

func (f *fakeEngine) Delete(_ context.Context, _ int, _ *credentials, ref objectRef) error {
	for _, id := range ref.Messages {
		f.deleted = append(f.deleted, strconv.Itoa(id))
	}
	return nil
}

func (f *fakeEngine) Download(_ context.Context, _ int, _ *credentials, _ objectRef, messageID int) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(string(f.files[messageID]))), nil
}
