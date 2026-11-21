package telegram

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"harbor/internal/store"
)

const (
	partBytes    int64 = 524288
	defaultParts       = 4000
	premiumParts       = 8000
	keyPrefix          = "tg:"
)

// DocumentLimit is one Telegram document: 4000 parts of 512 KB, or 8000 for Premium.
func DocumentLimit(premium bool) int64 {
	parts := int64(defaultParts)
	if premium {
		parts = premiumParts
	}
	return parts * partBytes
}

type credentials struct {
	APIHash       string      `json:"api_hash"`
	Session       []byte      `json:"session,omitempty"`
	PhoneCodeHash string      `json:"phone_code_hash,omitempty"`
	User          string      `json:"user,omitempty"`
	Authorized    bool        `json:"authorized,omitempty"`
	Premium       bool        `json:"premium,omitempty"`
	Peer          *storedPeer `json:"peer,omitempty"`
	// persist is set when this process changed the session and the handler
	// should seal it. A joined sendCode that gave up must not overwrite it.
	persist bool `json:"-"`
}

type storedPeer struct {
	Kind   string `json:"k"`
	ID     int64  `json:"id,omitempty"`
	Access int64  `json:"a,omitempty"`
	Target string `json:"t"`
}

type objectRef struct {
	Kind     string `json:"k"`
	ID       int64  `json:"id,omitempty"`
	Access   int64  `json:"a,omitempty"`
	Messages []int  `json:"m"`
}

func encodeCredentials(cred credentials) (string, error) {
	raw, err := json.Marshal(cred)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeCredentials(secret string) (credentials, error) {
	var cred credentials
	if err := json.Unmarshal([]byte(secret), &cred); err != nil || cred.APIHash == "" {
		return credentials{}, errors.New("telegram credentials are missing")
	}
	return cred, nil
}

// WithoutSession keeps the api hash and drops a saved login.
// A plain secret is returned unchanged.
func WithoutSession(secret string) (string, error) {
	if !strings.HasPrefix(strings.TrimSpace(secret), "{") {
		return secret, nil
	}
	cred, err := decodeCredentials(secret)
	if err != nil {
		return "", err
	}
	return encodeCredentials(credentials{APIHash: cred.APIHash})
}

// PublicStatus reports whether a destination is signed in, without its secrets.
func PublicStatus(secret string) (signedIn bool, label string) {
	cred, err := decodeCredentials(secret)
	if err != nil {
		return false, ""
	}
	return cred.Authorized && len(cred.Session) > 0, cred.User
}

// Prepare checks a Telegram destination and stores the api hash inside the sealed secret.
func Prepare(d store.Destination) (store.Destination, error) {
	d.AccountID = strings.TrimSpace(d.AccountID)
	d.AccessKeyID = strings.TrimSpace(d.AccessKeyID)
	d.Region = strings.TrimSpace(d.Region)
	d.Endpoint = strings.TrimSpace(d.Endpoint)
	d.Secret = strings.TrimSpace(d.Secret)
	d.Jurisdiction = ""
	d.AllowOverFree = false
	d.Bucket = "telegram"
	d.KeyPrefix = "telegram"
	phone, err := normalizePhone(d.AccountID)
	if err != nil {
		return store.Destination{}, err
	}
	d.AccountID = phone
	if _, err := strconv.Atoi(d.AccessKeyID); err != nil || strings.HasPrefix(d.AccessKeyID, "-") || d.AccessKeyID == "0" {
		return store.Destination{}, errors.New("api id is the number from my.telegram.org/apps")
	}
	target, err := normalizeTarget(d.Endpoint)
	if err != nil {
		return store.Destination{}, err
	}
	d.Endpoint = target
	if d.Region != "" && !validThread(d.Region) {
		return store.Destination{}, errors.New("topic id must be a positive number")
	}
	hash := d.Secret
	if strings.HasPrefix(hash, "{") {
		cred, err := decodeCredentials(hash)
		if err != nil {
			return store.Destination{}, err
		}
		hash = cred.APIHash
		if !validHash(hash) {
			return store.Destination{}, errors.New("api hash should be the hex value from my.telegram.org/apps")
		}
		return d, nil
	}
	if hash != "" && !validHash(hash) {
		return store.Destination{}, errors.New("api hash should be the hex value from my.telegram.org/apps")
	}
	if hash == "" {
		return d, nil
	}
	encoded, err := encodeCredentials(credentials{APIHash: hash})
	if err != nil {
		return store.Destination{}, err
	}
	d.Secret = encoded
	return d, nil
}

func normalizePhone(raw string) (string, error) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(raw) {
		switch {
		case r == '+' && i == 0:
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '(' || r == ')':
		default:
			return "", errors.New("phone number should look like +15551234567")
		}
	}
	digits := strings.TrimPrefix(b.String(), "+")
	if len(digits) < 7 || len(digits) > 15 {
		return "", errors.New("phone number should look like +15551234567")
	}
	return "+" + digits, nil
}

func validHash(hash string) bool {
	if len(hash) < 16 || len(hash) > 64 {
		return false
	}
	for _, r := range hash {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func validThread(raw string) bool {
	n, err := strconv.ParseInt(raw, 10, 64)
	return err == nil && n > 0
}

func normalizeTarget(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	switch strings.ToLower(raw) {
	case "", "me", "self", "saved", "saved messages":
		return "", nil
	}
	if strings.HasPrefix(raw, "@") {
		if !validUsername(raw[1:]) {
			return "", errors.New("channel username should look like @harborbackups")
		}
		return raw, nil
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n != 0 {
		return strconv.FormatInt(n, 10), nil
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "t.me/") || strings.Contains(lower, "telegram.me/") || strings.HasPrefix(lower, "tg://") {
		return raw, nil
	}
	if validUsername(raw) {
		return "@" + raw, nil
	}
	return "", errors.New("target should be empty for Saved Messages, or a @username, invite link, or numeric id")
}

func validUsername(name string) bool {
	if len(name) < 4 || len(name) > 32 {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func partCount(size, limit int64) int {
	if size <= 0 {
		return 1
	}
	if limit < 1 {
		limit = DocumentLimit(false)
	}
	n := int((size + limit - 1) / limit)
	if n < 1 {
		return 1
	}
	return n
}

func caption(project, name, kind, created, checksum string, size int64, part, count int) string {
	text := fmt.Sprintf("Harbor backup\nProject: %s\nName: %s\nKind: %s\nSize: %d bytes\nWhen: %s\nSHA-256: %s",
		oneLine(project), oneLine(name), oneLine(kind), size, oneLine(created), oneLine(checksum))
	if count > 1 {
		text += fmt.Sprintf("\nPart %d of %d. Download the parts in order and join them with cat. The SHA-256 is of the whole file.", part, count)
	}
	if len(text) > 1024 {
		text = text[:1024]
	}
	return text
}

func oneLine(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if s == "" {
		return "-"
	}
	return s
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
	if count == 1 {
		return "harbor-" + base + ".bin"
	}
	return fmt.Sprintf("harbor-%s-part%02d-of-%02d.bin", base, part, count)
}

func encodeKey(ref objectRef) (string, error) {
	raw, err := json.Marshal(ref)
	if err != nil {
		return "", err
	}
	return keyPrefix + string(raw), nil
}

func decodeKey(key string) (objectRef, error) {
	if !strings.HasPrefix(key, keyPrefix) {
		return objectRef{}, errors.New("not a telegram object")
	}
	var ref objectRef
	if err := json.Unmarshal([]byte(strings.TrimPrefix(key, keyPrefix)), &ref); err != nil || len(ref.Messages) == 0 {
		return objectRef{}, errors.New("telegram message ids are missing")
	}
	return ref, nil
}

func redact(err error, cred credentials) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if cred.APIHash != "" {
		msg = strings.ReplaceAll(msg, cred.APIHash, "api-hash")
	}
	if cred.PhoneCodeHash != "" {
		msg = strings.ReplaceAll(msg, cred.PhoneCodeHash, "code-hash")
	}
	if code := StatusCode(err); code != 502 {
		return statusErr(code, msg)
	}
	return errors.New(msg)
}
