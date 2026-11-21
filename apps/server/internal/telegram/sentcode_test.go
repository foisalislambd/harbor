package telegram

import (
	"context"
	"strings"
	"testing"

	"harbor/internal/store"

	"github.com/gotd/td/session"
	"github.com/gotd/td/tg"
)

func TestDescribeSentCode(t *testing.T) {
	app := &tg.AuthSentCode{Type: &tg.AuthSentCodeTypeApp{Length: 5}, PhoneCodeHash: "hash"}
	app.SetNextType(&tg.AuthCodeTypeSMS{})
	app.SetTimeout(60)
	got := describeSentCode(app)
	if got.Type != "app" || got.TypeName != "auth.sentCodeTypeApp" || got.NextType != "sms" || got.NextTypeName != "auth.codeTypeSms" || got.Timeout != 60 || got.Length != 5 {
		t.Fatalf("%+v", got)
	}
	if strings.Contains(got.Message, "auth.") || !strings.Contains(got.Message, "Telegram app") {
		t.Fatal(got.Message)
	}

	email := describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeEmailCode{EmailPattern: "a*@x.com", Length: 6}})
	if email.Type != "email" || email.EmailPattern != "a*@x.com" || !strings.Contains(email.Message, "a*@x.com") {
		t.Fatalf("%+v", email)
	}
	setup := describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeSetUpEmailRequired{}})
	if setup.Type != "setup_email" || setup.TypeName != "auth.sentCodeTypeSetUpEmailRequired" || setup.needsLoginCode() || !strings.Contains(setup.Message, "email address") {
		t.Fatalf("%+v", setup)
	}
	fragment := describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeFragmentSMS{URL: "https://fragment.com", Length: 6}})
	if fragment.Type != "fragment_sms" || fragment.TypeName != "auth.sentCodeTypeFragmentSms" || fragment.URL != "https://fragment.com" || strings.Contains(fragment.Message, "auth.") {
		t.Fatalf("%+v", fragment)
	}
	firebase := describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeFirebaseSMS{Length: 5}})
	if firebase.Type != "firebase_sms" || firebase.TypeName != "auth.sentCodeTypeFirebaseSms" || strings.Contains(firebase.Message, "auth.") {
		t.Fatalf("%+v", firebase)
	}
	if describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeCall{Length: 5}}).Type != "call" {
		t.Fatal("call")
	}
	if describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeFlashCall{Pattern: "123"}}).Type != "flash_call" {
		t.Fatal("flash")
	}
	if describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeMissedCall{Prefix: "12", Length: 4}}).Type != "missed_call" {
		t.Fatal("missed")
	}
	appOnly := describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeApp{Length: 5}, PhoneCodeHash: "hash"})
	if appOnly.NextType != "" || strings.Contains(appOnly.Message, "auth.") || strings.Contains(appOnly.Message, "next_type") {
		t.Fatal(appOnly.Message)
	}
}

func TestResendKeepsTheSession(t *testing.T) {
	raw, err := encodeCredentials(credentials{APIHash: "0123456789abcdef0123456789abcdef"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := Open(store.Destination{
		Provider: store.ProviderTelegram, AccountID: "+1 (555) 123-4567", AccessKeyID: "123456", Secret: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.engine = &fakeEngine{}
	if _, err := client.SendCode(context.Background()); err != nil {
		t.Fatal(err)
	}
	session := string(client.cred.Session)
	resent, err := client.ResendCode(context.Background())
	if err != nil || resent.Type != "sms" || client.cred.PhoneCodeHash != "resent-hash" || string(client.cred.Session) != session {
		t.Fatalf("%+v %v session %q", resent, err, client.cred.Session)
	}
	pattern, length, err := client.SendLoginEmail(context.Background(), "ada@example.com")
	if err != nil || pattern != "a*@example.com" || length != 6 {
		t.Fatalf("email %s %d %v", pattern, length, err)
	}
	if _, _, err := client.SendLoginEmail(context.Background(), "not-an-email"); err == nil {
		t.Fatal("expected an email error")
	}
	if err := client.CancelCode(context.Background()); err != nil || client.cred.PhoneCodeHash != "" {
		t.Fatal(err)
	}
}

func TestSentCodeRequiresTypeAndHash(t *testing.T) {
	if _, err := applySentCode(&credentials{}, &tg.AuthSentCode{}); err == nil {
		t.Fatal("expected a missing type")
	}
	if _, err := applySentCode(&credentials{}, &tg.AuthSentCode{Type: &tg.AuthSentCodeTypeApp{Length: 5}}); err == nil {
		t.Fatal("expected a missing phone_code_hash")
	}
}

func TestSessionDC(t *testing.T) {
	storage := &memorySession{}
	err := (&session.Loader{Storage: storage}).Save(context.Background(), &session.Data{DC: 4})
	if err != nil {
		t.Fatal(err)
	}
	if got := sessionDC(storage.snapshot()); got != 4 {
		t.Fatalf("dc %d", got)
	}
	if sessionDC(nil) != 0 || sessionDC([]byte("nope")) != 0 {
		t.Fatal("empty session")
	}
}
