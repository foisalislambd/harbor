package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetentionDropsOldestAndBlob(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	project, err := st.CreateProject(ctx, "Billing", "db dumps", 2, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	sums := []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	}
	var ids []string
	for i, sum := range sums {
		path, err := st.BlobPath(sum)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(sum), 0o644); err != nil {
			t.Fatal(err)
		}
		b, err := st.CreateBackup(ctx, Backup{
			ProjectID: project.ID, Name: sum[:1], Filename: "dump.bin", Kind: "database",
			SizeBytes: 4, Checksum: sum, ContentType: "application/octet-stream",
		})
		if err != nil {
			t.Fatal(err)
		}
		stamp := "2024-01-0" + string(rune('1'+i)) + "T00:00:00Z"
		if _, err := st.db.ExecContext(ctx, `UPDATE backups SET created_at = ? WHERE id = ?`, stamp, b.ID); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, b.ID)
	}

	removed, err := st.EnforceRetention(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].ID != ids[0] {
		t.Fatalf("removed %#v", removed)
	}
	if _, err := os.Stat(mustPath(t, st, sums[0])); !os.IsNotExist(err) {
		t.Fatalf("oldest blob still present: %v", err)
	}
	if _, err := os.Stat(mustPath(t, st, sums[2])); err != nil {
		t.Fatal(err)
	}
	items, total, err := st.ListBackups(ctx, BackupFilter{ProjectID: project.ID})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("total %d items %d", total, len(items))
	}
}

func TestKeepsNewestRowWhenTimestampsMatch(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	project, err := st.CreateProject(ctx, "Same second", "", 1, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	sumA := strings.Repeat("a1", 32)
	sumB := strings.Repeat("b2", 32)
	writeBlob(t, st, sumA)
	writeBlob(t, st, sumB)
	first, err := st.CreateBackup(ctx, Backup{ProjectID: project.ID, Name: "first", Filename: "a.bin", Kind: "archive", SizeBytes: 1, Checksum: sumA, ContentType: "application/octet-stream"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateBackup(ctx, Backup{ProjectID: project.ID, Name: "second", Filename: "b.bin", Kind: "archive", SizeBytes: 1, Checksum: sumB, ContentType: "application/octet-stream"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE backups SET created_at = ? WHERE project_id = ?`, "2024-06-01T00:00:00Z", project.ID); err != nil {
		t.Fatal(err)
	}
	removed, err := st.EnforceRetention(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0].ID != first.ID {
		t.Fatalf("removed %#v, wanted first %s", removed, first.ID)
	}
	if _, err := st.GetBackup(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
}

func TestDuplicateContentCountsOnceTowardQuota(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	project, err := st.CreateProject(ctx, "Quota", "", 0, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	sumA := strings.Repeat("aa", 32)
	sumB := strings.Repeat("bb", 32)
	if _, err := st.CreateBackup(ctx, Backup{ProjectID: project.ID, Name: "a", Filename: "a.bin", Kind: "archive", SizeBytes: 8, Checksum: sumA, ContentType: "application/octet-stream"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateBackup(ctx, Backup{ProjectID: project.ID, Name: "a-again", Filename: "a.bin", Kind: "archive", SizeBytes: 8, Checksum: sumA, ContentType: "application/octet-stream"}); err != nil {
		t.Fatal(err)
	}
	used, err := st.Usage(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if used != 8 {
		t.Fatalf("used %d", used)
	}
	_, err = st.CreateBackup(ctx, Backup{ProjectID: project.ID, Name: "b", Filename: "b.bin", Kind: "archive", SizeBytes: 8, Checksum: sumB, ContentType: "application/octet-stream"})
	if !errors.Is(err, ErrQuota) {
		t.Fatalf("expected quota error, got %v", err)
	}
}

func TestSharedBlobRemovedOnlyAfterLastReference(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	project, err := st.CreateProject(ctx, "Share", "", 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	sum := strings.Repeat("cd", 32)
	writeBlob(t, st, sum)
	first, err := st.CreateBackup(ctx, Backup{ProjectID: project.ID, Name: "one", Filename: "a.bin", Kind: "files", SizeBytes: 2, Checksum: sum, ContentType: "application/octet-stream"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateBackup(ctx, Backup{ProjectID: project.ID, Name: "two", Filename: "a.bin", Kind: "files", SizeBytes: 2, Checksum: sum, ContentType: "application/octet-stream"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteBackup(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mustPath(t, st, sum)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteBackup(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mustPath(t, st, sum)); !os.IsNotExist(err) {
		t.Fatalf("blob still present: %v", err)
	}
}

func openStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func writeBlob(t *testing.T, st *Store, sum string) {
	t.Helper()
	path, err := st.BlobPath(sum)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(sum), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustPath(t *testing.T, st *Store, sum string) string {
	t.Helper()
	path, err := st.BlobPath(sum)
	if err != nil {
		t.Fatal(err)
	}
	return path
}
