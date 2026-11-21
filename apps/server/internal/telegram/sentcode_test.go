package telegram

import (
	"context"
	"strings"
	"testing"

	"harbor/internal/store"

	"github.com/gotd/td/tg"
)

func TestDescribeSentCode(t *testing.T) {
	app := &tg.AuthSentCode{Type: &tg.AuthSentCodeTypeApp{Length: 5}, PhoneCodeHash: "hash"}
	app.SetNextType(&tg.AuthCodeTypeSMS{})
	app.SetTimeout(60)
	got := describeSentCode(app)
	if got.Type != "app" || got.URL != "" || strings.Contains(got.Message, "auth.") || !strings.Contains(got.Message, "Telegram app") {
		t.Fatalf("%+v", got)
	}

	email := describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeEmailCode{EmailPattern: "a*@x.com", Length: 6}})
	if email.Type != "email" || !strings.Contains(email.Message, "a*@x.com") {
		t.Fatalf("%+v", email)
	}
	setup := describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeSetUpEmailRequired{}})
	if setup.Type != "setup_email" || setup.needsLoginCode() || !strings.Contains(setup.Message, "email address") {
		t.Fatalf("%+v", setup)
	}
	fragment := describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeFragmentSMS{URL: "https://fragment.com", Length: 6}})
	if fragment.Type != "fragment_sms" || fragment.URL != "https://fragment.com" || strings.Contains(fragment.Message, "auth.") {
		t.Fatalf("%+v", fragment)
	}
	firebase := describeSentCode(&tg.AuthSentCode{Type: &tg.AuthSentCodeTypeFirebaseSMS{Length: 5}})
	if firebase.Type != "firebase_sms" || strings.Contains(firebase.Message, "auth.") {
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
}

func TestLoginEmailValidation(t *testing.T) {
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
	if _, err := client.SendCode(context.Background()); err != nil || client.cred.PhoneCodeHash == "" {
		t.Fatal(err)
	}
	pattern, length, err := client.SendLoginEmail(context.Background(), "ada@example.com")
	if err != nil || pattern != "a*@example.com" || length != 6 {
		t.Fatalf("email %s %d %v", pattern, length, err)
	}
	if _, _, err := client.SendLoginEmail(context.Background(), "not-an-email"); err == nil {
		t.Fatal("expected an email error")
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
