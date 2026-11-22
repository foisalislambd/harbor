package secretbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSealRoundTripAndLegacyPlaintext(t *testing.T) {
	dir := t.TempDir()
	box, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secret.key mode %o", info.Mode().Perm())
	}
	sealed, err := box.Seal("destination-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !Sealed(sealed) || sealed == "destination-secret" {
		t.Fatalf("sealed %q", sealed)
	}
	got, err := box.Open(sealed)
	if err != nil || got != "destination-secret" {
		t.Fatalf("open %q %v", got, err)
	}
	legacy, err := box.Open("still-plaintext")
	if err != nil || legacy != "still-plaintext" {
		t.Fatalf("legacy %q %v", legacy, err)
	}
}

func TestEnvKeyIsNotWrittenToDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HARBOR_SECRET_KEY", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	box, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "secret.key")); !os.IsNotExist(err) {
		t.Fatalf("env key should not create a file, stat err=%v", err)
	}
	sealed, err := box.Seal("webhook-secret")
	if err != nil {
		t.Fatal(err)
	}
	again, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := again.Open(sealed)
	if err != nil || got != "webhook-secret" {
		t.Fatalf("reopen %q %v", got, err)
	}
}
