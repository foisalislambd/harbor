// Package telegram copies a backup into a Telegram chat with the Bot API.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"harbor/internal/copier"
	"harbor/internal/netguard"
	"harbor/internal/store"
)

const (
	// DefaultAPI is the public Bot API. Uploads there are limited to 50 MB
	// and downloads through getFile to 20 MB, so parts stay at 20 MB.
	DefaultAPI = "https://api.telegram.org"
	// PublicPart is one document on the public Bot API.
	PublicPart int64 = 20 << 20
	// LocalPart is one document on a custom Bot API server, under its 2 GB cap.
	LocalPart int64 = 1900 << 20
)

// Client talks to one bot and one chat.
type Client struct {
	API       string
	Token     string
	ChatID    string
	ThreadID  string
	Username  string
	HTTP      *http.Client
	PartSize  int64
	transport http.RoundTripper
}

// Prepare checks a Telegram destination and fills the columns the S3 form uses.
func Prepare(d store.Destination) (store.Destination, error) {
	d.AccountID = strings.TrimSpace(d.AccountID)
	d.Region = strings.TrimSpace(d.Region)
	d.Endpoint = strings.TrimRight(strings.TrimSpace(d.Endpoint), "/")
	d.Secret = strings.TrimSpace(d.Secret)
	d.Jurisdiction = ""
	d.AllowOverFree = false
	d.Bucket = "telegram"
	d.KeyPrefix = "telegram"
	if !validChat(d.AccountID) {
		return store.Destination{}, errors.New("enter a chat id such as -1001234567890 or @channelname")
	}
	if d.Region != "" && !validThread(d.Region) {
		return store.Destination{}, errors.New("topic id must be a positive number")
	}
	if d.Secret != "" && !validToken(d.Secret) {
		return store.Destination{}, errors.New("bot token should look like 123456789:AAH...")
	}
	if d.Endpoint != "" {
		if err := checkAPI(d.Endpoint); err != nil {
			return store.Destination{}, err
		}
	}
	if d.AccessKeyID == "" {
		d.AccessKeyID = "bot"
	}
	return d, nil
}

// Open builds a client. The token must already be present.
func Open(d store.Destination) (*Client, error) {
	prepared, err := Prepare(d)
	if err != nil {
		return nil, err
	}
	if !validToken(prepared.Secret) {
		return nil, errors.New("bot token should look like 123456789:AAH...")
	}
	api := prepared.Endpoint
	if api == "" {
		api = DefaultAPI
	}
	return &Client{
		API: api, Token: prepared.Secret, ChatID: prepared.AccountID,
		ThreadID: prepared.Region, Username: prepared.AccessKeyID,
	}, nil
}

func checkAPI(raw string) error {
	if err := netguard.CheckURL(raw); err != nil {
		if errors.Is(err, netguard.ErrBlocked) {
			return errors.New("bot api url cannot point at a link-local or metadata address")
		}
		return err
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("bot api url must be an http or https origin")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("bot api url must start with http:// or https://")
	}
	return nil
}

func validToken(token string) bool {
	id, secret, ok := strings.Cut(token, ":")
	if !ok || id == "" || secret == "" || len(token) > 256 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	for _, r := range secret {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func validChat(chat string) bool {
	if strings.HasPrefix(chat, "@") {
		name := chat[1:]
		if len(name) < 5 || len(name) > 32 {
			return false
		}
		for _, r := range name {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
				return false
			}
		}
		return true
	}
	if chat == "" || len(chat) > 20 {
		return false
	}
	for i, r := range chat {
		if r == '-' && i == 0 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return chat != "-"
}

func validThread(raw string) bool {
	n, err := strconv.ParseInt(raw, 10, 64)
	return err == nil && n > 0
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	if c.transport == nil {
		c.transport = &http.Transport{
			Proxy:               nil,
			DialContext:         netguard.DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			IdleConnTimeout:     90 * time.Second,
		}
	}
	return &http.Client{Transport: c.transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func (c *Client) partSize() int64 {
	if c.PartSize > 0 {
		return c.PartSize
	}
	if c.API == DefaultAPI {
		return PublicPart
	}
	return LocalPart
}

// Check calls getMe and getChat, then posts and removes a short test message.
func (c *Client) Check(ctx context.Context) error {
	var me struct {
		Username string `json:"username"`
		ID       int64  `json:"id"`
	}
	if err := c.call(ctx, "getMe", nil, nil, &me); err != nil {
		return err
	}
	if me.Username != "" {
		c.Username = me.Username
	} else if me.ID != 0 {
		c.Username = fmt.Sprintf("bot%d", me.ID)
	}
	if err := c.call(ctx, "getChat", url.Values{"chat_id": {c.ChatID}}, nil, nil); err != nil {
		return err
	}
	var sent struct {
		MessageID int64 `json:"message_id"`
	}
	fields := url.Values{
		"chat_id": {c.ChatID},
		"text":    {"Harbor is connected to this chat."},
	}
	if c.ThreadID != "" {
		fields.Set("message_thread_id", c.ThreadID)
	}
	if err := c.call(ctx, "sendMessage", fields, nil, &sent); err != nil {
		return err
	}
	if sent.MessageID != 0 {
		_ = c.call(ctx, "deleteMessage", url.Values{
			"chat_id":    {c.ChatID},
			"message_id": {strconv.FormatInt(sent.MessageID, 10)},
		}, nil, nil)
	}
	return nil
}

// Put sends the file as one or more documents and returns the message ids.
func (c *Client) Put(ctx context.Context, up copier.Upload) (string, error) {
	if up.Size < 0 {
		return "", errors.New("backup size is invalid")
	}
	part := c.partSize()
	count := int((up.Size + part - 1) / part)
	if up.Size == 0 {
		count = 1
	}
	file, err := os.Open(up.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	refs := make([]partRef, 0, count)
	for i := 0; i < count; i++ {
		n := part
		if up.Size > 0 {
			remain := up.Size - int64(i)*part
			if remain < n {
				n = remain
			}
		} else {
			n = 0
		}
		filename := documentName(up.Name, i+1, count)
		caption := caption(up, i+1, count)
		ref, err := c.sendDocument(ctx, file, int64(i)*part, n, filename, caption)
		if err != nil {
			return "", err
		}
		refs = append(refs, ref)
	}
	raw, err := json.Marshal(refs)
	if err != nil {
		return "", err
	}
	return "tg:" + string(raw), nil
}

// Delete removes every message stored in the key. A message Telegram has
// already dropped is treated as gone.
func (c *Client) Delete(ctx context.Context, key string) error {
	refs, err := parseKey(key)
	if err != nil {
		return err
	}
	var failed []string
	for _, ref := range refs {
		err := c.call(ctx, "deleteMessage", url.Values{
			"chat_id":    {c.ChatID},
			"message_id": {strconv.FormatInt(ref.MessageID, 10)},
		}, nil, nil)
		if err == nil || alreadyGone(err) {
			continue
		}
		failed = append(failed, err.Error())
	}
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	return nil
}

// Open downloads each part and concatenates them. The public Bot API only
// returns files up to 20 MB; a custom server that answers with a disk path
// cannot be read from here.
func (c *Client) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	refs, err := parseKey(key)
	if err != nil {
		return nil, err
	}
	pipes := make([]io.Reader, 0, len(refs))
	closers := make([]io.Closer, 0, len(refs))
	for _, ref := range refs {
		var file struct {
			FilePath string `json:"file_path"`
		}
		if err := c.call(ctx, "getFile", url.Values{"file_id": {ref.FileID}}, nil, &file); err != nil {
			closeAll(closers)
			return nil, err
		}
		if file.FilePath == "" || strings.HasPrefix(file.FilePath, "/") || strings.Contains(file.FilePath, "..") {
			closeAll(closers)
			return nil, errors.New("telegram did not return a downloadable file; save the parts from the chat in order and join them")
		}
		body, err := c.download(ctx, file.FilePath)
		if err != nil {
			closeAll(closers)
			return nil, err
		}
		pipes = append(pipes, body)
		closers = append(closers, body)
	}
	return &multiRead{Reader: io.MultiReader(pipes...), closers: closers}, nil
}

type partRef struct {
	MessageID int64  `json:"m"`
	FileID    string `json:"f"`
}

func parseKey(key string) ([]partRef, error) {
	if !strings.HasPrefix(key, "tg:") {
		return nil, errors.New("not a telegram object")
	}
	var refs []partRef
	if err := json.Unmarshal([]byte(strings.TrimPrefix(key, "tg:")), &refs); err != nil || len(refs) == 0 {
		return nil, errors.New("telegram message ids are missing")
	}
	return refs, nil
}

func alreadyGone(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "message to delete not found") || strings.Contains(msg, "message identifier is not specified")
}

func documentName(name string, part, count int) string {
	clean := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			clean = append(clean, r)
		default:
			clean = append(clean, '-')
		}
	}
	base := strings.Trim(string(clean), "-")
	if base == "" {
		base = "backup"
	}
	if len(base) > 40 {
		base = base[:40]
	}
	return fmt.Sprintf("harbor-%s-part%02d-of-%02d.bin", base, part, count)
}

func caption(up copier.Upload, part, count int) string {
	text := fmt.Sprintf("Harbor backup\nProject: %s\nName: %s\nKind: %s\nSize: %d bytes\nWhen: %s\nSHA-256: %s\nPart %d of %d. Download the parts in order and join them with cat. The SHA-256 is of the whole file.",
		oneLine(up.Project), oneLine(up.Name), oneLine(up.Kind), up.Size, oneLine(up.CreatedAt), oneLine(up.Checksum), part, count)
	if len(text) > 1000 {
		text = text[:1000]
	}
	return text
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	return s
}

func (c *Client) sendDocument(ctx context.Context, file *os.File, offset, n int64, filename, caption string) (partRef, error) {
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			if err := wait(ctx, last); err != nil {
				return partRef{}, err
			}
		}
		ref, retry, err := c.sendDocumentOnce(ctx, file, offset, n, filename, caption)
		if err == nil {
			return ref, nil
		}
		last = err
		if !retry {
			return partRef{}, redact(err, c.Token)
		}
	}
	return partRef{}, redact(last, c.Token)
}

func (c *Client) sendDocumentOnce(ctx context.Context, file *os.File, offset, n int64, filename, caption string) (partRef, bool, error) {
	body, length, boundary, err := documentBody(file, offset, n, filename, c.ChatID, c.ThreadID, caption)
	if err != nil {
		return partRef{}, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.method("sendDocument"), body)
	if err != nil {
		return partRef{}, false, err
	}
	req.ContentLength = length
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	req.Header.Set("User-Agent", "Harbor")
	resp, err := c.client().Do(req)
	if err != nil {
		return partRef{}, true, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return partRef{}, true, err
	}
	var envelope struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		ErrorCode   int    `json:"error_code"`
		Parameters  struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
		Result struct {
			MessageID int64 `json:"message_id"`
			Document  struct {
				FileID string `json:"file_id"`
			} `json:"document"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return partRef{}, resp.StatusCode >= 500, fmt.Errorf("telegram sendDocument: %s", resp.Status)
	}
	if !envelope.OK {
		err := &apiError{Code: envelope.ErrorCode, Description: envelope.Description, RetryAfter: envelope.Parameters.RetryAfter}
		return partRef{}, err.temporary(), err
	}
	if envelope.Result.MessageID == 0 || envelope.Result.Document.FileID == "" {
		return partRef{}, false, errors.New("telegram did not return a message id")
	}
	return partRef{MessageID: envelope.Result.MessageID, FileID: envelope.Result.Document.FileID}, false, nil
}

func documentBody(file *os.File, offset, n int64, filename, chatID, threadID, caption string) (io.Reader, int64, string, error) {
	var scratch bytes.Buffer
	w := multipart.NewWriter(&scratch)
	if err := w.WriteField("chat_id", chatID); err != nil {
		return nil, 0, "", err
	}
	if threadID != "" {
		if err := w.WriteField("message_thread_id", threadID); err != nil {
			return nil, 0, "", err
		}
	}
	if err := w.WriteField("caption", caption); err != nil {
		return nil, 0, "", err
	}
	if _, err := w.CreateFormFile("document", filename); err != nil {
		return nil, 0, "", err
	}
	header := append([]byte(nil), scratch.Bytes()...)
	footer := []byte("\r\n--" + w.Boundary() + "--\r\n")
	section := io.NewSectionReader(file, offset, n)
	return io.MultiReader(bytes.NewReader(header), section, bytes.NewReader(footer)), int64(len(header)) + n + int64(len(footer)), w.Boundary(), nil
}

type apiError struct {
	Code        int
	Description string
	RetryAfter  int
}

func (e *apiError) Error() string {
	if e.Description == "" {
		return fmt.Sprintf("telegram error %d", e.Code)
	}
	return e.Description
}

func (e *apiError) temporary() bool {
	return e.Code == 429 || e.Code >= 500
}

func wait(ctx context.Context, cause error) error {
	delay := 2 * time.Second
	var api *apiError
	if errors.As(cause, &api) && api.RetryAfter > 0 {
		delay = time.Duration(api.RetryAfter) * time.Second
	}
	if delay > 60*time.Second {
		return errors.New("telegram asked to wait too long")
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) call(ctx context.Context, method string, fields url.Values, _ any, out any) error {
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			if err := wait(ctx, last); err != nil {
				return err
			}
		}
		var body io.Reader
		if fields != nil {
			body = strings.NewReader(fields.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.method(method), body)
		if err != nil {
			return err
		}
		if fields != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader(fields.Encode())), nil
			}
		}
		req.Header.Set("User-Agent", "Harbor")
		resp, err := c.client().Do(req)
		if err != nil {
			last = err
			continue
		}
		payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if err != nil {
			last = err
			continue
		}
		var envelope struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
			ErrorCode   int    `json:"error_code"`
			Parameters  struct {
				RetryAfter int `json:"retry_after"`
			} `json:"parameters"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			if resp.StatusCode >= 500 {
				last = fmt.Errorf("telegram %s: %s", method, resp.Status)
				continue
			}
			return redact(fmt.Errorf("telegram %s: %s", method, resp.Status), c.Token)
		}
		if !envelope.OK {
			api := &apiError{Code: envelope.ErrorCode, Description: envelope.Description, RetryAfter: envelope.Parameters.RetryAfter}
			if api.temporary() {
				last = api
				continue
			}
			return redact(api, c.Token)
		}
		if out != nil && len(envelope.Result) > 0 && string(envelope.Result) != "null" {
			if err := json.Unmarshal(envelope.Result, out); err != nil {
				return err
			}
		}
		return nil
	}
	return redact(last, c.Token)
}

func (c *Client) download(ctx context.Context, filePath string) (io.ReadCloser, error) {
	url := c.API + "/file/bot" + c.Token + "/" + filePath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Harbor")
	resp, err := c.client().Do(req)
	if err != nil {
		return nil, redact(err, c.Token)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("telegram file download: %s", resp.Status)
	}
	return resp.Body, nil
}

func (c *Client) method(name string) string {
	return c.API + "/bot" + c.Token + "/" + name
}

func redact(err error, token string) error {
	if err == nil || token == "" {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), token, "bot-token"))
}

type multiRead struct {
	io.Reader
	closers []io.Closer
}

func (m *multiRead) Close() error {
	return closeAll(m.closers)
}

func closeAll(closers []io.Closer) error {
	var err error
	for _, c := range closers {
		if c == nil {
			continue
		}
		if closeErr := c.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}
