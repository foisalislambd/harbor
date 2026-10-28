package r2

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSigV4MatchesAWSExample(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "examplebucket.s3.amazonaws.com"
	req.Header.Set("Range", "bytes=0-9")
	req.Header.Set("x-amz-content-sha256", emptyHash)
	req.Header.Set("x-amz-date", "20130524T000000Z")
	now := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	got := authorization(req, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", "s3", emptyHash, now)
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got != want {
		t.Fatalf("signature mismatch:\n got %s\nwant %s", got, want)
	}
}

func TestEndpointURL(t *testing.T) {
	got, err := EndpointURL("0123456789abcdef0123456789ABCDEF", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://0123456789abcdef0123456789abcdef.r2.cloudflarestorage.com" {
		t.Fatal(got)
	}
	got, err = EndpointURL("0123456789abcdef0123456789abcdef", "eu")
	if err != nil || !strings.Contains(got, ".eu.r2.cloudflarestorage.com") {
		t.Fatal(got, err)
	}
	if _, err := EndpointURL("not-an-account", "fedramp"); err == nil {
		t.Fatal("expected invalid account id")
	}
}

func TestPutDeleteAndMultipart(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		if r.Header.Get("Authorization") == "" || r.Header.Get("X-Amz-Date") == "" {
			t.Errorf("missing signature headers on %s", r.URL.RequestURI())
		}
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.RawQuery, "uploads"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><UploadId>up-1</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPut && r.URL.Query().Get("partNumber") != "":
			w.Header().Set("ETag", `"part-`+r.URL.Query().Get("partNumber")+`"`)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Query().Get("uploadId") != "":
			body, _ := io.ReadAll(r.Body)
			if !bytes.Contains(body, []byte("<PartNumber>1</PartNumber>")) || !bytes.Contains(body, []byte(`"part-1"`)) {
				t.Errorf("complete body %s", body)
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut:
			sum := sha256.Sum256([]byte("hello"))
			if r.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(sum[:]) {
				t.Errorf("payload hash %s", r.Header.Get("X-Amz-Content-Sha256"))
			}
			if r.Header.Get("X-Amz-Storage-Class") != "STANDARD" {
				t.Errorf("storage class %q", r.Header.Get("X-Amz-Storage-Class"))
			}
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	client := &Client{
		Endpoint: srv.URL, Bucket: "backups", AccessKey: "AKIAIOSFODNN7EXAMPLE",
		Secret: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", Region: "auto", HTTP: srv.Client(),
	}
	ctx := context.Background()
	sum := sha256.Sum256([]byte("hello"))
	path := writeTemp(t, []byte("hello"))
	if err := client.PutFile(ctx, "harbor/blobs/abc", path, "application/octet-stream", hex.EncodeToString(sum[:]), 5); err != nil {
		t.Fatal(err)
	}
	if err := client.Delete(ctx, "harbor/blobs/abc"); err != nil {
		t.Fatal(err)
	}

	prevPart, prevSingle := multipartPartSize, singleUploadMax
	multipartPartSize = 4
	singleUploadMax = 0
	t.Cleanup(func() {
		multipartPartSize = prevPart
		singleUploadMax = prevSingle
	})
	big := writeTemp(t, []byte("0123456789"))
	bigSum := sha256.Sum256([]byte("0123456789"))
	if err := client.PutFile(ctx, "harbor/blobs/big", big, "application/octet-stream", hex.EncodeToString(bigSum[:]), 10); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "PUT /backups/harbor/blobs/abc") {
		t.Fatalf("single put path missing:\n%s", joined)
	}
	if !strings.Contains(joined, "DELETE /backups/harbor/blobs/abc") {
		t.Fatalf("delete path missing:\n%s", joined)
	}
	if strings.Count(joined, "partNumber=") != 3 {
		t.Fatalf("expected 3 parts:\n%s", joined)
	}
}

func TestMultipartAbortsWhenAPartFails(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.RawQuery)
		if strings.Contains(r.URL.RawQuery, "uploads") {
			_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><UploadId>up-9</UploadId></InitiateMultipartUploadResult>`)
			return
		}
		if r.URL.Query().Get("partNumber") == "2" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `<Error><Code>InternalError</Code><Message>nope</Message></Error>`)
			return
		}
		w.Header().Set("ETag", `"ok"`)
	}))
	defer srv.Close()
	prevPart, prevSingle := multipartPartSize, singleUploadMax
	multipartPartSize = 4
	singleUploadMax = 0
	t.Cleanup(func() {
		multipartPartSize = prevPart
		singleUploadMax = prevSingle
	})
	client := &Client{Endpoint: srv.URL, Bucket: "backups", AccessKey: "AKIAiosfodnn7example", Secret: "secretsecretsecret", HTTP: srv.Client()}
	err := client.PutFile(context.Background(), "k", writeTemp(t, []byte("01234567")), "application/octet-stream", strings.Repeat("ab", 32), 8)
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(methods, "\n"), "DELETE") {
		t.Fatalf("multipart was not aborted: %v", methods)
	}
}

func writeTemp(t *testing.T, body []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "obj")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}
