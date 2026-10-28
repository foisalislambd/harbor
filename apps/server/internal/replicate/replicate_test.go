package replicate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"harbor/internal/remote/r2"
	"harbor/internal/store"
)

func TestSyncStaysLocalWhenDestinationMissingAndCopiesWhenSet(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	project := makeProject(t, st)
	body := []byte("hello harbor")
	sum := sha256.Sum256(body)
	checksum := hex.EncodeToString(sum[:])
	writeBlob(t, st, checksum, body)
	backup := makeBackup(t, st, project.ID, checksum, int64(len(body)))

	status, err := SyncBackup(ctx, st, backup)
	if err != nil || status != "" {
		t.Fatalf("local only: status %q err %v", status, err)
	}

	var puts, deletes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			puts++
			got, _ := io.ReadAll(r.Body)
			if !strings.Contains(r.URL.Path, "/backups/harbor/blobs/"+checksum) || string(got) != string(body) {
				t.Errorf("put %s (%d bytes)", r.URL.Path, len(got))
			}
		case http.MethodDelete:
			deletes++
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	restore := stubClient(t, srv)
	defer restore()

	dest := makeDestination(t, st)
	project.DestinationID = dest.ID
	if _, err := st.UpdateProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	if status, err := SyncBackup(ctx, st, backup); err != nil || status != "copied" {
		t.Fatalf("copy: status %q err %v", status, err)
	}
	if status, err := SyncBackup(ctx, st, backup); err != nil || status != "already" {
		t.Fatalf("second copy: status %q err %v", status, err)
	}
	if puts != 1 {
		t.Fatalf("puts = %d", puts)
	}

	other := makeBackup(t, st, project.ID, checksum, int64(len(body)))
	if status, err := SyncBackup(ctx, st, other); err != nil || status != "copied" {
		t.Fatalf("shared blob: status %q err %v", status, err)
	}
	if puts != 1 {
		t.Fatalf("shared blob uploaded again: puts = %d", puts)
	}
	if _, err := st.DeleteBackup(ctx, backup.ID); err != nil {
		t.Fatal(err)
	}
	FlushDeletes(ctx, st)
	if deletes != 0 {
		t.Fatalf("deleted shared object early: %d", deletes)
	}
	if _, err := st.DeleteBackup(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	FlushDeletes(ctx, st)
	if deletes != 1 {
		t.Fatalf("deletes = %d", deletes)
	}
}

func TestFreeStorageIsSharedByOneCloudflareAccount(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	prev := storageLimit
	storageLimit = func(string) int64 { return 6 }
	t.Cleanup(func() { storageLimit = prev })

	body := []byte("hello!")
	sum := sha256.Sum256(body)
	checksum := hex.EncodeToString(sum[:])
	writeBlob(t, st, checksum, body)
	otherBody := []byte("second")
	otherSum := sha256.Sum256(otherBody)
	otherChecksum := hex.EncodeToString(otherSum[:])
	writeBlob(t, st, otherChecksum, otherBody)

	first := makeProject(t, st)
	second, err := st.CreateProject(ctx, "Notes", "", 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	one := makeDestination(t, st)
	two, err := st.CreateDestination(ctx, store.Destination{
		Name: "Other bucket", Provider: store.ProviderCloudflareR2,
		AccountID: one.AccountID, AccessKeyID: "AKIAIOSFODNN7EXAMPLE",
		Secret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", Bucket: "notes", KeyPrefix: "harbor",
	})
	if err != nil {
		t.Fatal(err)
	}
	first.DestinationID = one.ID
	second.DestinationID = two.ID
	if _, err := st.UpdateProject(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateProject(ctx, second); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	restore := stubClient(t, srv)
	defer restore()

	kept := makeBackup(t, st, first.ID, checksum, int64(len(body)))
	if status, err := SyncBackup(ctx, st, kept); err != nil || status != "copied" {
		t.Fatalf("first copy: %q %v", status, err)
	}
	extra := makeBackup(t, st, second.ID, otherChecksum, int64(len(otherBody)))
	if status, err := SyncBackup(ctx, st, extra); err != nil || status != "skipped" {
		t.Fatalf("second account copy: %q %v", status, err)
	}
}

func TestSyncStopsAtFreeStorage(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	project := makeProject(t, st)
	dest := makeDestination(t, st)
	project.DestinationID = dest.ID
	if _, err := st.UpdateProject(ctx, project); err != nil {
		t.Fatal(err)
	}
	prev := storageLimit
	storageLimit = func(string) int64 { return 4 }
	t.Cleanup(func() { storageLimit = prev })
	body := []byte("hello")
	sum := sha256.Sum256(body)
	checksum := hex.EncodeToString(sum[:])
	writeBlob(t, st, checksum, body)
	backup := makeBackup(t, st, project.ID, checksum, int64(len(body)))
	status, err := SyncBackup(ctx, st, backup)
	if err != nil || status != "skipped" {
		t.Fatalf("status %q err %v", status, err)
	}
	fresh, err := st.GetBackup(ctx, backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.Copies) != 1 || fresh.Copies[0].Status != "skipped" || !strings.Contains(fresh.Copies[0].Error, "4 bytes") {
		t.Fatalf("copies %+v", fresh.Copies)
	}
}

func stubClient(t *testing.T, srv *httptest.Server) func() {
	t.Helper()
	prev := openClient
	openClient = func(d store.Destination) (*r2.Client, error) {
		return &r2.Client{
			Endpoint: srv.URL, Bucket: d.Bucket, AccessKey: d.AccessKeyID, Secret: d.Secret,
			Region: "auto", HTTP: srv.Client(),
		}, nil
	}
	return func() { openClient = prev }
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func makeProject(t *testing.T, st *store.Store) store.Project {
	t.Helper()
	project, err := st.CreateProject(context.Background(), "Ledger", "", 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return project
}

func makeDestination(t *testing.T, st *store.Store) store.Destination {
	t.Helper()
	dest, err := st.CreateDestination(context.Background(), store.Destination{
		Name: "Cloudflare", Provider: store.ProviderCloudflareR2,
		AccountID: "0123456789abcdef0123456789abcdef", AccessKeyID: "AKIAIOSFODNN7EXAMPLE",
		Secret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", Bucket: "backups", KeyPrefix: "harbor",
	})
	if err != nil {
		t.Fatal(err)
	}
	return dest
}

func makeBackup(t *testing.T, st *store.Store, projectID, checksum string, size int64) store.Backup {
	t.Helper()
	backup, err := st.CreateBackup(context.Background(), store.Backup{
		ProjectID: projectID, Name: "nightly", Filename: "dump.bin", Kind: "archive",
		SizeBytes: size, Checksum: checksum, ContentType: "application/octet-stream",
	})
	if err != nil {
		t.Fatal(err)
	}
	return backup
}

func writeBlob(t *testing.T, st *store.Store, checksum string, body []byte) {
	t.Helper()
	path, err := st.BlobPath(checksum)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepathDir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func filepathDir(path string) string {
	i := strings.LastIndexAny(path, `/\`)
	if i < 0 {
		return "."
	}
	return path[:i]
}
