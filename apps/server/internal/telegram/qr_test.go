package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func TestQRTokenURLAndRefresh(t *testing.T) {
	token := qrlogin.NewToken([]byte{0x00, 0xff, 0x10}, int(time.Now().Add(30*time.Second).Unix()))
	if !strings.HasPrefix(token.URL(), "tg://login?token=") || strings.Contains(token.URL(), " ") {
		t.Fatal(token.URL())
	}
	image, err := qrPNG(token)
	if err != nil || !strings.HasPrefix(image, "data:image/png;base64,") {
		t.Fatalf("png %v %q", err, image)
	}
	pending := newQRPending()
	pending.storeToken(token.URL(), image, token.Expires())
	if _, ok := pending.cached(); !ok {
		t.Fatal("fresh token should stay on screen")
	}
	pending.markLogged()
	if _, ok := pending.cached(); ok {
		t.Fatal("accepted token must be exported again")
	}
	pending.takeLogged()
	pending.storeToken(token.URL(), image, time.Now().Add(3*time.Second))
	if _, ok := pending.cached(); ok {
		t.Fatal("token near expiry must refresh")
	}
	if !loginTokenUpdate(&tg.Updates{Updates: []tg.UpdateClass{&tg.UpdateLoginToken{}}}) {
		t.Fatal("update")
	}
}

func TestFinishQRMigrateAndPassword(t *testing.T) {
	var moved int
	authz := &tg.AuthAuthorization{User: &tg.User{ID: 7, FirstName: "Ada"}}
	got, err := finishQR(context.Background(), qrlogin.ExportResult{
		MigrateTo: &tg.AuthLoginTokenMigrateTo{DCID: 5, Token: []byte("move")},
	}, func(_ context.Context, dc int) error {
		moved = dc
		return nil
	}, func(_ context.Context, token []byte) (tg.AuthLoginTokenClass, error) {
		if string(token) != "move" || moved != 5 {
			t.Fatalf("import token %q dc %d", token, moved)
		}
		return &tg.AuthLoginTokenSuccess{Authorization: authz}, nil
	})
	if err != nil || got.Account == "" {
		t.Fatalf("migrate %+v %v", got, err)
	}
	_, err = finishQR(context.Background(), qrlogin.ExportResult{
		MigrateTo: &tg.AuthLoginTokenMigrateTo{DCID: 5, Token: []byte("move")},
	}, func(context.Context, int) error { return nil }, func(context.Context, []byte) (tg.AuthLoginTokenClass, error) {
		return nil, tgerr.New(401, "SESSION_PASSWORD_NEEDED")
	})
	if !errorsIsPassword(err) {
		t.Fatal(err)
	}
	shown, err := finishQR(context.Background(), qrlogin.ExportResult{
		Token: qrlogin.NewToken([]byte("abc"), int(time.Now().Add(time.Minute).Unix())),
	}, nil, nil)
	if err != nil || !strings.HasPrefix(shown.URL, "tg://login?token=") || shown.Image == "" {
		t.Fatalf("show %+v %v", shown, err)
	}
}

func TestStartQRReplacesUnsignedSession(t *testing.T) {
	fake := &fakeEngine{}
	client := &Client{apiID: 1, phone: "+15551234567", cred: credentials{APIHash: "0123456789abcdef0123456789abcdef", Session: []byte("old"), PhoneCodeHash: "hash"}, engine: fake}
	status, err := client.StartQR(context.Background())
	if err != nil || !fake.qrFresh || status.Status != "qr" || !strings.HasPrefix(status.URL, "tg://login?token=") {
		t.Fatalf("start %+v %v fresh %v", status, err, fake.qrFresh)
	}
	if string(client.cred.Session) != "qr-key" || client.cred.PhoneCodeHash != "" || !client.PersistSession() || client.cred.Authorized {
		t.Fatalf("session %+v", client.cred)
	}
	fake.qrDone = true
	done, err := client.PollQR(context.Background())
	if err != nil || done.Status != "ok" || !client.cred.Authorized || client.cred.User != "Ada (@ada)" {
		t.Fatalf("poll %+v %v %+v", done, err, client.cred)
	}
	other := &Client{apiID: 1, phone: "+15551234567", cred: credentials{APIHash: "0123456789abcdef0123456789abcdef"}, engine: fake}
	fake.qrPassword = true
	fake.qrDone = false
	password, err := other.StartQR(context.Background())
	if err != nil || password.Status != "password" || other.cred.Authorized {
		t.Fatalf("password %+v %v authorized %v", password, err, other.cred.Authorized)
	}
}
