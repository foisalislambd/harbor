package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"harbor/internal/secretbox"
)

func TestPlaintextSecretsAreSealedOnOpen(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := st.db.ExecContext(ctx, `INSERT INTO destinations (id, name, provider, account_id, access_key_id, secret_access_key, bucket, jurisdiction, region, endpoint, allow_over_free, key_prefix, created_at, updated_at) VALUES ('dst_old', 'Old', 'minio', '', 'access-key', 'plaintext-secret', 'backups', '', '', 'http://127.0.0.1:9000', 0, 'harbor', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `INSERT INTO projects (id, name, slug, description, created_at, updated_at) VALUES ('prj_old', 'Old', 'old', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `INSERT INTO webhooks (id, project_id, url, secret, events, created_at) VALUES ('wh_old', 'prj_old', 'https://example.com/hook', 'plain-hook', 'backup.created', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	st.Close()

	st, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	var stored string
	if err := st.db.QueryRowContext(ctx, `SELECT secret_access_key FROM destinations WHERE id = 'dst_old'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !secretbox.Sealed(stored) || strings.Contains(stored, "plaintext-secret") {
		t.Fatalf("destination secret was not sealed: %s", stored)
	}
	dest, err := st.GetDestination(ctx, "dst_old")
	if err != nil || dest.Secret != "plaintext-secret" {
		t.Fatalf("decrypted destination secret %q %v", dest.Secret, err)
	}
	hooks, err := st.ListWebhooks(ctx, "prj_old")
	if err != nil || len(hooks) != 1 || hooks[0].Secret != "plain-hook" {
		t.Fatalf("hooks %+v %v", hooks, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "secret.key")); err != nil {
		t.Fatal(err)
	}
}

func TestDataDirWithQuestionMarkStillOpens(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a?b")
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.UserCount(context.Background()); err != nil {
		t.Fatal(err)
	}
}
