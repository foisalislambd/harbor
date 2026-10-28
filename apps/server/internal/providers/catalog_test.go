package providers

import (
	"strings"
	"testing"

	"harbor/internal/store"
)

func TestEveryProviderBuildsAnEndpoint(t *testing.T) {
	for _, provider := range All() {
		dest := store.Destination{
			Provider:    provider.ID,
			AccountID:   "0123456789abcdef0123456789abcdef",
			Region:      provider.RegionPlaceholder,
			Endpoint:    provider.EndpointPlaceholder,
			Bucket:      "harbor-backups",
			AccessKeyID: "access-key",
			Secret:      "secret-key",
		}
		if provider.ID == "supabase" {
			dest.Endpoint = "https://project.supabase.co/storage/v1/s3"
		}
		if provider.ID == "minio" || provider.ID == "ceph" || provider.ID == "garage" || provider.ID == "seaweedfs" {
			dest.Endpoint = provider.EndpointPlaceholder
		}
		got, err := Prepare(dest)
		if err != nil {
			t.Fatalf("%s: %v", provider.ID, err)
		}
		client, err := Open(got)
		if err != nil {
			t.Fatalf("%s: %v", provider.ID, err)
		}
		if !strings.HasPrefix(client.Endpoint, "https://") && !strings.HasPrefix(client.Endpoint, "http://") {
			t.Fatalf("%s endpoint %s", provider.ID, client.Endpoint)
		}
		if provider.ID == "backblaze_b2" && (!client.Versioned || client.StorageClass != "") {
			t.Fatalf("b2 should delete versions and skip storage class: %+v", client)
		}
		if provider.ID == store.ProviderCloudflareR2 && client.StorageClass != "STANDARD" {
			t.Fatal(client.StorageClass)
		}
		if provider.ID == "alibaba" && (client.Endpoint != "https://s3.oss-cn-hangzhou.aliyuncs.com" || !client.VirtualHost || client.Region != "cn-hangzhou") {
			t.Fatalf("alibaba client %+v", client)
		}
		if provider.ID == "tencent" && !client.VirtualHost {
			t.Fatal("tencent needs the bucket in the host")
		}
		if provider.ID == "contabo" && (client.Endpoint != "https://eu2.contabostorage.com" || client.Region != "default") {
			t.Fatalf("contabo client %+v", client)
		}
	}
}

func TestUnknownProviderAndBadEndpoint(t *testing.T) {
	if _, err := Prepare(store.Destination{Provider: "google_drive"}); err == nil {
		t.Fatal("expected unknown provider")
	}
	got, err := Prepare(store.Destination{Provider: "amazon_s3", Region: "us-east-1", Endpoint: "http://evil.example"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := Open(got)
	if err != nil {
		t.Fatal(err)
	}
	if client.Endpoint != "https://s3.us-east-1.amazonaws.com" {
		t.Fatal(client.Endpoint)
	}
}
