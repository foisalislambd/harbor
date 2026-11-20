package netguard

import "testing"

func TestCheckURLBlocksMetadataAndAllowsLoopback(t *testing.T) {
	if err := CheckURL("http://169.254.169.254/latest/meta-data"); err == nil {
		t.Fatal("expected metadata address to be blocked")
	}
	if err := CheckURL("http://metadata.google.internal/computeMetadata/v1/"); err == nil {
		t.Fatal("expected metadata hostname to be blocked")
	}
	if err := CheckURL("http://127.0.0.1:9000"); err != nil {
		t.Fatal(err)
	}
	if err := CheckURL("http://user:pass@127.0.0.1:9000"); err == nil {
		t.Fatal("expected userinfo to be rejected")
	}
}
