// Package r2 talks to Cloudflare R2 through its S3-compatible API.
// Objects are stored as Standard. R2's free monthly allowance covers Standard
// storage only, so this client never sets a storage class.
package r2

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"harbor/internal/store"
)

const (
	// FreeStorageBytes is the Standard storage included with R2 each month (10 GB).
	FreeStorageBytes int64 = 10_000_000_000
	// FreeClassAMonthly is the included writes, lists of writes, and deletes.
	FreeClassAMonthly = 1_000_000
	// FreeClassBMonthly is the included reads.
	FreeClassBMonthly = 10_000_000
)

var (
	multipartPartSize int64 = 8 << 20
	singleUploadMax   int64 = 100 << 20
)

type completedPart struct {
	number int
	etag   string
}

type Client struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	Secret    string
	Region    string
	HTTP      *http.Client
	Now       func() time.Time
}

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return fmt.Sprintf("Cloudflare R2 returned HTTP %d", e.Status)
}

type xmlError struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// EndpointURL is the S3 API host for an account. EU and FedRAMP buckets use a
// jurisdiction-specific host. Other bucket locations keep the default host.
func EndpointURL(accountID, jurisdiction string) (string, error) {
	accountID = strings.ToLower(strings.TrimSpace(accountID))
	if len(accountID) != 32 {
		return "", errors.New("account ID must be the 32-character ID from the Cloudflare dashboard")
	}
	for _, r := range accountID {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return "", errors.New("account ID must be the 32-character ID from the Cloudflare dashboard")
		}
	}
	switch jurisdiction {
	case "", "default":
		return "https://" + accountID + ".r2.cloudflarestorage.com", nil
	case "eu":
		return "https://" + accountID + ".eu.r2.cloudflarestorage.com", nil
	case "fedramp":
		return "https://" + accountID + ".fedramp.r2.cloudflarestorage.com", nil
	default:
		return "", errors.New("jurisdiction must be default, eu, or fedramp")
	}
}

func FromDestination(d store.Destination) (*Client, error) {
	endpoint, err := EndpointURL(d.AccountID, d.Jurisdiction)
	if err != nil {
		return nil, err
	}
	return &Client{
		Endpoint:  endpoint,
		Bucket:    d.Bucket,
		AccessKey: d.AccessKeyID,
		Secret:    d.Secret,
		Region:    "auto",
	}, nil
}

var defaultHTTP = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 2 * time.Minute,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 0,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return defaultHTTP
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
}

func (c *Client) region() string {
	if c.Region != "" {
		return c.Region
	}
	return "auto"
}

// Check confirms the bucket exists and this key can list it. An Object Read &
// Write token can list objects. It cannot call HeadBucket.
func (c *Client) Check(ctx context.Context) error {
	query := url.Values{}
	query.Set("list-type", "2")
	query.Set("max-keys", "1")
	resp, err := c.do(ctx, http.MethodGet, "", query, nil, 0, emptyHash, "", false)
	if err != nil {
		var api *apiError
		if errors.As(err, &api) && (api.Status == http.StatusNotFound || api.Code == "NoSuchBucket") {
			return errors.New("Cloudflare R2 does not have that bucket in this jurisdiction")
		}
		if errors.As(err, &api) && (api.Status == http.StatusForbidden || api.Status == http.StatusUnauthorized) {
			return errors.New("Cloudflare R2 refused this access key for that bucket")
		}
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return nil
}

// PutFile stores path at key. Files larger than 100 MiB use multipart upload.
// The caller passes the SHA-256 of the whole file for the single-part request.
func (c *Client) PutFile(ctx context.Context, key, path, contentType, sha string, size int64) error {
	if !useMultipart(size) {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		resp, err := c.do(ctx, http.MethodPut, key, nil, f, size, sha, contentType, true)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}
	return c.putMultipart(ctx, key, path, contentType, size)
}

func useMultipart(size int64) bool {
	const maxSingle = 5<<30 - 5<<20
	if size >= maxSingle {
		return true
	}
	return size > singleUploadMax
}

func (c *Client) putMultipart(ctx context.Context, key, path, contentType string, size int64) error {
	uploadID, err := c.createMultipart(ctx, key, contentType)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		c.abort(key, uploadID)
		return err
	}
	defer f.Close()
	var parts []completedPart
	var offset int64
	number := 1
	for offset < size {
		n := multipartPartSize
		if remain := size - offset; remain < n {
			n = remain
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(io.NewSectionReader(f, offset, n), buf); err != nil {
			c.abort(key, uploadID)
			return err
		}
		sum := sha256.Sum256(buf)
		etag, err := c.uploadPart(ctx, key, uploadID, number, buf, hex.EncodeToString(sum[:]))
		if err != nil {
			c.abort(key, uploadID)
			return err
		}
		parts = append(parts, completedPart{number: number, etag: etag})
		offset += n
		number++
	}
	if err := c.completeMultipart(ctx, key, uploadID, parts); err != nil {
		c.abort(key, uploadID)
		return err
	}
	return nil
}

func (c *Client) createMultipart(ctx context.Context, key, contentType string) (string, error) {
	query := url.Values{}
	query.Set("uploads", "")
	resp, err := c.do(ctx, http.MethodPost, key, query, bytes.NewReader(nil), 0, emptyHash, contentType, true)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", err
	}
	if result.UploadID == "" {
		return "", errors.New("Cloudflare R2 did not return an upload id")
	}
	return result.UploadID, nil
}

func (c *Client) uploadPart(ctx context.Context, key, uploadID string, number int, body []byte, sha string) (string, error) {
	query := url.Values{}
	query.Set("partNumber", fmt.Sprintf("%d", number))
	query.Set("uploadId", uploadID)
	resp, err := c.do(ctx, http.MethodPut, key, query, bytes.NewReader(body), int64(len(body)), sha, "", false)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	etag := resp.Header.Get("ETag")
	if etag == "" {
		return "", errors.New("Cloudflare R2 did not return a part checksum")
	}
	return etag, nil
}

func (c *Client) completeMultipart(ctx context.Context, key, uploadID string, parts []completedPart) error {
	var buf bytes.Buffer
	buf.WriteString("<CompleteMultipartUpload>")
	for _, part := range parts {
		buf.WriteString("<Part><PartNumber>")
		buf.WriteString(fmt.Sprintf("%d", part.number))
		buf.WriteString("</PartNumber><ETag>")
		buf.WriteString(escapeXMLText(part.etag))
		buf.WriteString("</ETag></Part>")
	}
	buf.WriteString("</CompleteMultipartUpload>")
	sum := sha256.Sum256(buf.Bytes())
	query := url.Values{}
	query.Set("uploadId", uploadID)
	resp, err := c.do(ctx, http.MethodPost, key, query, bytes.NewReader(buf.Bytes()), int64(buf.Len()), hex.EncodeToString(sum[:]), "application/xml", false)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) abort(key, uploadID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	query := url.Values{}
	query.Set("uploadId", uploadID)
	resp, err := c.do(ctx, http.MethodDelete, key, query, nil, 0, emptyHash, "", false)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// Delete removes one object. A missing object is success.
func (c *Client) Delete(ctx context.Context, key string) error {
	resp, err := c.doRaw(ctx, http.MethodDelete, key, nil, nil, 0, emptyHash, "", false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return readAPIError(resp)
}

// Open downloads an object.
func (c *Client) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := c.do(ctx, http.MethodGet, key, nil, nil, 0, emptyHash, "", false)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) do(ctx context.Context, method, key string, query url.Values, body io.Reader, size int64, payloadHash, contentType string, standardClass bool) (*http.Response, error) {
	resp, err := c.doRaw(ctx, method, key, query, body, size, payloadHash, contentType, standardClass)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, readAPIError(resp)
	}
	return resp, nil
}

func (c *Client) doRaw(ctx context.Context, method, key string, query url.Values, body io.Reader, size int64, payloadHash, contentType string, standardClass bool) (*http.Response, error) {
	if payloadHash == "" {
		payloadHash = emptyHash
	}
	endpoint := strings.TrimRight(c.Endpoint, "/")
	u, err := url.Parse(endpoint + objectPath(c.Bucket, key))
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		u.RawQuery = canonicalQuery(query)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Host = u.Host
	req.ContentLength = size
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if standardClass {
		// A bucket can default to Infrequent Access, which is outside the free tier.
		req.Header.Set("x-amz-storage-class", "STANDARD")
	}
	now := c.now().UTC()
	req.Header.Set("x-amz-date", now.Format("20060102T150405Z"))
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("Authorization", authorization(req, c.AccessKey, c.Secret, c.region(), "s3", payloadHash, now))
	req.Header.Set("User-Agent", "harbor")
	return c.client().Do(req)
}

func objectPath(bucket, key string) string {
	path := "/" + escape(bucket)
	key = strings.Trim(key, "/")
	if key == "" {
		return path
	}
	for _, part := range strings.Split(key, "/") {
		path += "/" + escape(part)
	}
	return path
}

func readAPIError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	api := &apiError{Status: resp.StatusCode}
	var body xmlError
	if xml.Unmarshal(raw, &body) == nil {
		api.Code = strings.TrimSpace(body.Code)
		api.Message = strings.TrimSpace(body.Message)
	}
	if api.Message == "" && len(raw) > 0 && !bytes.Contains(raw, []byte("<")) {
		api.Message = strings.TrimSpace(string(raw))
	}
	return api
}

const emptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func authorization(req *http.Request, access, secret, region, service, payloadHash string, now time.Time) string {
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := amzDate[:8]
	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	headers, signed := canonicalHeaders(req)
	canonical := strings.Join([]string{
		req.Method,
		escapePath(req.URL.EscapedPath()),
		canonicalQuery(req.URL.Query()),
		headers,
		signed,
		payloadHash,
	}, "\n")
	sum := sha256.Sum256([]byte(canonical))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	signing := hmacSHA256(hmacSHA256(hmacSHA256(hmacSHA256([]byte("AWS4"+secret), dateStamp), region), service), "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(signing, stringToSign))
	return "AWS4-HMAC-SHA256 Credential=" + access + "/" + scope + ", SignedHeaders=" + signed + ", Signature=" + signature
}

func canonicalHeaders(req *http.Request) (string, string) {
	keys := []string{"host"}
	for key := range req.Header {
		lower := strings.ToLower(key)
		if lower == "authorization" || lower == "user-agent" {
			continue
		}
		keys = append(keys, lower)
	}
	sort.Strings(keys)
	var lines []string
	for _, key := range keys {
		var value string
		if key == "host" {
			value = req.Host
			if value == "" {
				value = req.URL.Host
			}
		} else {
			value = strings.Join(req.Header.Values(http.CanonicalHeaderKey(key)), ",")
		}
		value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		lines = append(lines, key+":"+value)
	}
	return strings.Join(lines, "\n") + "\n", strings.Join(keys, ";")
}

func canonicalQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var parts []string
	for _, key := range keys {
		vals := append([]string(nil), values[key]...)
		sort.Strings(vals)
		ek := escape(key)
		if len(vals) == 0 {
			parts = append(parts, ek+"=")
			continue
		}
		for _, value := range vals {
			parts = append(parts, ek+"="+escape(value))
		}
	}
	return strings.Join(parts, "&")
}

func escapePath(path string) string {
	if path == "" {
		return "/"
	}
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		unescaped, err := url.PathUnescape(part)
		if err != nil {
			unescaped = part
		}
		parts[i] = escape(unescaped)
	}
	return strings.Join(parts, "/")
}

func escapeXMLText(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(data))
	return mac.Sum(nil)
}
