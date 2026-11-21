package telegram

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"harbor/internal/copier"
	"harbor/internal/store"
)

// ErrPasswordNeeded means the code was accepted and a cloud password is still required.
var ErrPasswordNeeded = errors.New("telegram cloud password required")

type engine interface {
	SendCode(ctx context.Context, apiID int, cred *credentials, phone string) (CodeStatus, error)
	ResendCode(ctx context.Context, apiID int, cred *credentials, phone string) (CodeStatus, error)
	CancelCode(ctx context.Context, apiID int, cred *credentials, phone string) error
	SendLoginEmail(ctx context.Context, apiID int, cred *credentials, phone, email string) (string, int, error)
	VerifyLoginEmail(ctx context.Context, apiID int, cred *credentials, phone, code string) (CodeStatus, error)
	SignIn(ctx context.Context, apiID int, cred *credentials, phone, code string) (string, error)
	Password(ctx context.Context, apiID int, cred *credentials, phone, password string) (string, error)
	LogOut(ctx context.Context, apiID int, cred *credentials, phone string) error
	Self(ctx context.Context, apiID int, cred *credentials) (string, bool, error)
	Resolve(ctx context.Context, apiID int, cred *credentials, target string) (storedPeer, error)
	Upload(ctx context.Context, apiID int, cred *credentials, peer storedPeer, topic int, file *os.File, offset, n int64, filename, caption string) (int, error)
	Delete(ctx context.Context, apiID int, cred *credentials, ref objectRef) error
	Download(ctx context.Context, apiID int, cred *credentials, ref objectRef, messageID int) (io.ReadCloser, error)
}

// Client is one signed-in Telegram account and one target chat.
type Client struct {
	apiID  int
	phone  string
	target string
	topic  int
	cred   credentials
	engine engine
	limit  int64
}

// Open reads a saved destination. It does not connect.
func Open(d store.Destination) (*Client, error) {
	prepared, err := Prepare(d)
	if err != nil {
		return nil, err
	}
	cred, err := decodeCredentials(prepared.Secret)
	if err != nil {
		return nil, err
	}
	apiID, err := strconv.Atoi(prepared.AccessKeyID)
	if err != nil || apiID < 1 {
		return nil, errors.New("api id is the number from my.telegram.org/apps")
	}
	topic := 0
	if prepared.Region != "" {
		topic, err = strconv.Atoi(prepared.Region)
		if err != nil || topic < 1 {
			return nil, errors.New("topic id must be a positive number")
		}
	}
	return &Client{
		apiID: apiID, phone: prepared.AccountID, target: prepared.Endpoint,
		topic: topic, cred: cred, engine: gotdEngine{},
	}, nil
}

// Secret is the credential JSON to seal again after a call that refreshed the session.
func (c *Client) Secret() string {
	raw, err := encodeCredentials(c.cred)
	if err != nil {
		return ""
	}
	return raw
}

// SendCode asks Telegram to deliver a login code. One account has one
// in-flight send, so a second click waits for that auth key instead of
// replacing it. PersistSession reports whether this call owns the session
// that should be sealed.
func (c *Client) SendCode(ctx context.Context) (CodeStatus, error) {
	c.cred.persist = false
	// Drop a leftover unauthorized key before the engine sees it. A signed-in
	// session is revoked by the one send that owns the request, not here,
	// because a second click must not log the account out twice.
	if !c.cred.Authorized {
		c.cred.Session = nil
		c.cred.PhoneCodeHash = ""
		c.cred.User = ""
		c.cred.Peer = nil
	}
	return c.engine.SendCode(ctx, c.apiID, &c.cred, c.phone)
}

// PersistSession reports whether SendCode changed the session on this call.
func (c *Client) PersistSession() bool {
	return c.cred.persist
}

// ResendCode asks Telegram for the next delivery method on the same auth key.
func (c *Client) ResendCode(ctx context.Context) (CodeStatus, error) {
	return c.engine.ResendCode(ctx, c.apiID, &c.cred, c.phone)
}

// CancelCode drops the pending login code and its connection.
func (c *Client) CancelCode(ctx context.Context) error {
	if err := c.engine.CancelCode(ctx, c.apiID, &c.cred, c.phone); err != nil {
		return err
	}
	c.cred.PhoneCodeHash = ""
	return nil
}

// SendLoginEmail starts the login-email check Telegram requires before a code.
func (c *Client) SendLoginEmail(ctx context.Context, email string) (string, int, error) {
	email = strings.TrimSpace(email)
	if !strings.Contains(email, "@") || strings.Contains(email, " ") {
		return "", 0, errors.New("enter an email address")
	}
	return c.engine.SendLoginEmail(ctx, c.apiID, &c.cred, c.phone, email)
}

// VerifyLoginEmail confirms that email and returns where the login code went.
func (c *Client) VerifyLoginEmail(ctx context.Context, code string) (CodeStatus, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return CodeStatus{}, errors.New("enter the email code")
	}
	return c.engine.VerifyLoginEmail(ctx, c.apiID, &c.cred, c.phone, code)
}

// SignIn completes the code step. ErrPasswordNeeded means the cloud password is next.
func (c *Client) SignIn(ctx context.Context, code string) (string, error) {
	if c.cred.PhoneCodeHash == "" {
		return "", errSendCodeFirst
	}
	name, err := c.engine.SignIn(ctx, c.apiID, &c.cred, c.phone, code)
	if err != nil {
		return "", err
	}
	c.cred.Authorized = true
	c.cred.PhoneCodeHash = ""
	c.cred.User = name
	return name, nil
}

// Password finishes a 2FA sign-in.
func (c *Client) Password(ctx context.Context, password string) (string, error) {
	name, err := c.engine.Password(ctx, c.apiID, &c.cred, c.phone, password)
	if err != nil {
		return "", err
	}
	c.cred.Authorized = true
	c.cred.PhoneCodeHash = ""
	c.cred.User = name
	return name, nil
}

// LogOut revokes the MTProto session.
func (c *Client) LogOut(ctx context.Context) error {
	if err := c.engine.LogOut(ctx, c.apiID, &c.cred, c.phone); err != nil {
		return err
	}
	c.cred.Session = nil
	c.cred.Authorized = false
	c.cred.User = ""
	c.cred.PhoneCodeHash = ""
	c.cred.Peer = nil
	c.cred.Premium = false
	return nil
}

// Check confirms the session and that the target chat can be resolved.
func (c *Client) Check(ctx context.Context) error {
	if !c.cred.Authorized || len(c.cred.Session) == 0 {
		return errNotSignedIn
	}
	name, premium, err := c.engine.Self(ctx, c.apiID, &c.cred)
	if err != nil {
		return err
	}
	c.cred.User = name
	c.cred.Premium = premium
	peer, err := c.engine.Resolve(ctx, c.apiID, &c.cred, c.target)
	if err != nil {
		return err
	}
	c.cred.Peer = &peer
	return nil
}

// Put uploads the backup as one document, or numbered parts above the account limit.
func (c *Client) Put(ctx context.Context, up copier.Upload) (string, error) {
	if err := c.Check(ctx); err != nil {
		return "", err
	}
	limit := DocumentLimit(c.cred.Premium)
	if c.limit > 0 {
		limit = c.limit
	}
	count := partCount(up.Size, limit)
	file, err := os.Open(up.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	peer := *c.cred.Peer
	ids := make([]int, 0, count)
	for i := 0; i < count; i++ {
		n := limit
		offset := int64(i) * limit
		if up.Size > 0 {
			remain := up.Size - offset
			if remain < n {
				n = remain
			}
		} else {
			n = 0
		}
		id, err := c.engine.Upload(ctx, c.apiID, &c.cred, peer, c.topic, file, offset, n, documentName(up.Name, i+1, count), caption(up.Project, up.Name, up.Kind, up.CreatedAt, up.Checksum, up.Size, i+1, count))
		if err != nil {
			_ = c.engine.Delete(ctx, c.apiID, &c.cred, objectRef{Kind: peer.Kind, ID: peer.ID, Access: peer.Access, Messages: ids})
			return "", err
		}
		ids = append(ids, id)
	}
	return encodeKey(objectRef{Kind: peer.Kind, ID: peer.ID, Access: peer.Access, Messages: ids})
}

// Delete removes the messages for a backup. A message Telegram already dropped is gone.
func (c *Client) Delete(ctx context.Context, key string) error {
	ref, err := decodeKey(key)
	if err != nil {
		return err
	}
	if !c.cred.Authorized {
		return errNotSignedIn
	}
	return c.engine.Delete(ctx, c.apiID, &c.cred, ref)
}

// Open downloads every part in order.
func (c *Client) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	ref, err := decodeKey(key)
	if err != nil {
		return nil, err
	}
	if !c.cred.Authorized {
		return nil, errNotSignedIn
	}
	pr, pw := io.Pipe()
	go func() {
		var first error
		for _, id := range ref.Messages {
			body, err := c.engine.Download(ctx, c.apiID, &c.cred, ref, id)
			if err != nil {
				first = err
				break
			}
			_, err = io.Copy(pw, body)
			body.Close()
			if err != nil {
				first = err
				break
			}
		}
		_ = pw.CloseWithError(first)
	}()
	return pr, nil
}
