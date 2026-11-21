package telegram

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"rsc.io/qr"
)

// QRStatus is one step of a QR login. Status is "qr", "password", or "ok".
// URL is tg://login?token=... and is safe to render as a QR image. The token
// itself is not a session.
type QRStatus struct {
	Status    string `json:"status"`
	URL       string `json:"url,omitempty"`
	Expires   int64  `json:"expires,omitempty"`
	ExpiresIn int    `json:"expires_in,omitempty"`
	Image     string `json:"image,omitempty"`
	Account   string `json:"account_label,omitempty"`
	Message   string `json:"message,omitempty"`
}

// qrRefreshBefore is how early a poll replaces a token that is about to expire.
// Calling auth.exportLoginToken on every poll would mint a new token and
// invalidate the code on screen. A poll re-exports only when the phone accepted
// it (updateLoginToken) or this much life is left.
const qrRefreshBefore = 5 * time.Second

// qrPending is the token currently on screen for one held connection.
type qrPending struct {
	mu       sync.Mutex
	url      string
	expires  time.Time
	image    string
	account  string
	message  string
	password bool
	done     bool
	logged   atomic.Bool
}

func newQRPending() *qrPending {
	return &qrPending{}
}

func (q *qrPending) markLogged() {
	q.logged.Store(true)
}

func (q *qrPending) takeLogged() bool {
	return q.logged.Swap(false)
}

// cached returns the last result when another export would only rotate the token.
func (q *qrPending) cached() (QRStatus, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.done || q.password {
		return q.viewLocked(), true
	}
	if q.url != "" && time.Until(q.expires) > qrRefreshBefore && !q.logged.Load() {
		return q.viewLocked(), true
	}
	return QRStatus{}, false
}

func (q *qrPending) viewLocked() QRStatus {
	status := QRStatus{URL: q.url, Image: q.image, Account: q.account, Message: q.message}
	switch {
	case q.done:
		status.Status = "ok"
	case q.password:
		status.Status = "password"
		status.Message = "Enter your cloud password."
	default:
		status.Status = "qr"
		status.Expires = q.expires.Unix()
		left := int(time.Until(q.expires).Seconds())
		if left < 0 {
			left = 0
		}
		status.ExpiresIn = left
		status.Message = "Scan this code with Telegram."
	}
	return status
}

func (q *qrPending) storeToken(url, image string, expires time.Time) QRStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.url = url
	q.image = image
	q.expires = expires
	q.done = false
	q.password = false
	return q.viewLocked()
}

func (q *qrPending) storePassword() QRStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.password = true
	q.done = false
	return q.viewLocked()
}

func (q *qrPending) storeDone(account string) QRStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.done = true
	q.password = false
	q.account = account
	q.message = account + " is signed in."
	return q.viewLocked()
}

func loginTokenUpdate(u tg.UpdatesClass) bool {
	switch u := u.(type) {
	case *tg.Updates:
		return hasLoginToken(u.Updates)
	case *tg.UpdatesCombined:
		return hasLoginToken(u.Updates)
	case *tg.UpdateShort:
		_, ok := u.Update.(*tg.UpdateLoginToken)
		return ok
	default:
		return false
	}
}

func hasLoginToken(updates []tg.UpdateClass) bool {
	for _, update := range updates {
		if _, ok := update.(*tg.UpdateLoginToken); ok {
			return true
		}
	}
	return false
}

func (g gotdEngine) ExportQR(ctx context.Context, apiID int, cred *credentials, phone string, fresh bool) (QRStatus, error) {
	key := loginKey(apiID, phone, cred.APIHash)
	var ticket *qrPending
	if fresh {
		ticket = newQRPending()
	} else {
		hold := authSessions.get(key)
		if hold == nil || hold.qr == nil {
			return QRStatus{}, errQRFirst
		}
		if view, ok := hold.qr.cached(); ok {
			return view, nil
		}
		ticket = hold.qr
	}
	var status QRStatus
	err := g.holdCall(ctx, apiID, phone, cred, fresh, ticket, func(ctx context.Context, client *telegram.Client) error {
		if view, ok := ticket.cached(); ok {
			status = view
			return nil
		}
		ticket.takeLogged()
		next, err := pullQR(ctx, client, apiID, cred.APIHash)
		if errorsIsPassword(err) {
			status = ticket.storePassword()
			return ErrPasswordNeeded
		}
		if err != nil {
			return err
		}
		if next.Account != "" {
			cred.User = next.Account
			cred.Premium = next.premium
			status = ticket.storeDone(next.Account)
			return nil
		}
		status = ticket.storeToken(next.URL, next.Image, next.expires)
		return nil
	})
	if err == nil && status.Status == "ok" {
		authSessions.drop(key)
	}
	if errorsIsPassword(err) {
		return status, nil
	}
	return status, err
}

type qrPull struct {
	URL     string
	Image   string
	Account string
	expires time.Time
	premium bool
}

func pullQR(ctx context.Context, client *telegram.Client, apiID int, apiHash string) (qrPull, error) {
	qr := qrlogin.NewQR(client.API(), apiID, apiHash, qrlogin.Options{
		Migrate: client.MigrateTo,
	})
	result, err := qr.ExportResult(ctx)
	if err != nil {
		return qrPull{}, err
	}
	return finishQR(ctx, result, client.MigrateTo, func(ctx context.Context, token []byte) (tg.AuthLoginTokenClass, error) {
		return client.API().AuthImportLoginToken(ctx, token)
	})
}

func finishQR(ctx context.Context, result qrlogin.ExportResult, migrate func(context.Context, int) error, importToken func(context.Context, []byte) (tg.AuthLoginTokenClass, error)) (qrPull, error) {
	switch {
	case result.Authorization != nil:
		return authPull(result.Authorization), nil
	case result.MigrateTo != nil:
		if migrate == nil {
			return qrPull{}, errTelegramUnreachable
		}
		if err := migrate(ctx, result.MigrateTo.DCID); err != nil {
			return qrPull{}, err
		}
		imported, err := importToken(ctx, result.MigrateTo.Token)
		if err != nil {
			return qrPull{}, err
		}
		success, ok := imported.(*tg.AuthLoginTokenSuccess)
		if !ok {
			return qrPull{}, errTelegramUnreachable
		}
		authz, ok := success.Authorization.(*tg.AuthAuthorization)
		if !ok {
			return qrPull{}, errTelegramUnreachable
		}
		return authPull(authz), nil
	default:
		token := result.Token
		if token.Empty() {
			return qrPull{}, errQRFirst
		}
		imageURL, err := qrPNG(token)
		if err != nil {
			return qrPull{}, err
		}
		return qrPull{URL: token.URL(), Image: imageURL, expires: token.Expires()}, nil
	}
}

func authPull(authz *tg.AuthAuthorization) qrPull {
	if authz == nil {
		return qrPull{}
	}
	return qrPull{Account: userLabel(authz.User), premium: userPremium(authz.User)}
}

func errorsIsPassword(err error) bool {
	return tgerr.Is(err, "SESSION_PASSWORD_NEEDED") || errors.Is(err, ErrPasswordNeeded)
}

func qrPNG(token qrlogin.Token) (string, error) {
	img, err := token.Image(qr.M)
	if err != nil {
		return "", err
	}
	scaled := scaleImage(img, 8)
	var buf bytes.Buffer
	if err := png.Encode(&buf, scaled); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func scaleImage(src image.Image, factor int) image.Image {
	if factor < 1 {
		factor = 1
	}
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx()*factor, bounds.Dy()*factor))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			color := src.At(bounds.Min.X+x, bounds.Min.Y+y)
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					dst.Set(x*factor+dx, y*factor+dy, color)
				}
			}
		}
	}
	return dst
}
