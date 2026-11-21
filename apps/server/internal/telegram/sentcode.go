package telegram

import (
	"errors"
	"fmt"
	"log"

	"github.com/gotd/td/tg"
)

// CodeStatus is the auth.sentCode result the panel shows. It does not include
// the phone code hash.
type CodeStatus struct {
	// Type is a short name the panel uses to pick a form (app, setup_email, ...).
	Type string `json:"type"`
	// TypeName is the TL constructor, such as auth.sentCodeTypeSetUpEmailRequired.
	TypeName     string `json:"type_name,omitempty"`
	NextType     string `json:"next_type,omitempty"`
	NextTypeName string `json:"next_type_name,omitempty"`
	// DC is the datacenter the auth key was stored on after the call.
	DC           int    `json:"dc,omitempty"`
	Timeout      int    `json:"timeout,omitempty"`
	Length       int    `json:"length,omitempty"`
	EmailPattern string `json:"email_pattern,omitempty"`
	URL          string `json:"url,omitempty"`
	Message      string `json:"message"`
}

func (s CodeStatus) needsLoginCode() bool {
	switch s.Type {
	case "setup_email", "payment", "already", "":
		return false
	default:
		return true
	}
}

func applySentCode(cred *credentials, sent tg.AuthSentCodeClass) (CodeStatus, error) {
	switch code := sent.(type) {
	case *tg.AuthSentCode:
		if code.Type == nil {
			return CodeStatus{}, errors.New("telegram auth.sentCode did not include a type")
		}
		if code.PhoneCodeHash == "" {
			return CodeStatus{}, errors.New("telegram auth.sentCode did not include a phone_code_hash")
		}
		cred.PhoneCodeHash = code.PhoneCodeHash
		cred.Authorized = false
		status := describeSentCode(code)
		log.Printf("telegram auth.sentCode type=%s next_type=%s timeout=%d", status.TypeName, status.NextTypeName, status.Timeout)
		return status, nil
	case *tg.AuthSentCodeSuccess:
		if authz, ok := code.Authorization.(*tg.AuthAuthorization); ok {
			cred.User = userLabel(authz.User)
			cred.Premium = userPremium(authz.User)
			cred.Authorized = true
			cred.PhoneCodeHash = ""
		}
		status := CodeStatus{Type: "already", Message: "This Telegram account is already signed in."}
		log.Printf("telegram auth.sentCode type=%s", status.Type)
		return status, nil
	case *tg.AuthSentCodePaymentRequired:
		status := CodeStatus{
			Type:    "payment",
			Message: "Telegram requires a Premium purchase in an official app before this number can receive a login code. Harbor cannot complete that purchase.",
		}
		log.Printf("telegram auth.sentCode type=%s", status.Type)
		return status, nil
	default:
		return CodeStatus{}, fmt.Errorf("telegram returned %T instead of auth.sentCode", sent)
	}
}

func describeSentCode(code *tg.AuthSentCode) CodeStatus {
	status := CodeStatus{Type: sentTypeName(code.Type), TypeName: tlName(code.Type)}
	if next, ok := code.GetNextType(); ok {
		status.NextType = codeTypeName(next)
		status.NextTypeName = tlName(next)
	}
	if timeout, ok := code.GetTimeout(); ok {
		status.Timeout = timeout
	}
	status.Length = sentLength(code.Type)
	status.EmailPattern = sentEmailPattern(code.Type)
	status.URL = sentURL(code.Type)
	status.Message = sentMessage(status)
	return status
}

func sentTypeName(kind tg.AuthSentCodeTypeClass) string {
	switch kind.(type) {
	case *tg.AuthSentCodeTypeApp:
		return "app"
	case *tg.AuthSentCodeTypeSMS:
		return "sms"
	case *tg.AuthSentCodeTypeCall:
		return "call"
	case *tg.AuthSentCodeTypeFlashCall:
		return "flash_call"
	case *tg.AuthSentCodeTypeMissedCall:
		return "missed_call"
	case *tg.AuthSentCodeTypeFragmentSMS:
		return "fragment_sms"
	case *tg.AuthSentCodeTypeEmailCode:
		return "email"
	case *tg.AuthSentCodeTypeSetUpEmailRequired:
		return "setup_email"
	case *tg.AuthSentCodeTypeFirebaseSMS:
		return "firebase_sms"
	case *tg.AuthSentCodeTypeSMSWord:
		return "sms_word"
	case *tg.AuthSentCodeTypeSMSPhrase:
		return "sms_phrase"
	default:
		return "unknown"
	}
}

func codeTypeName(kind tg.AuthCodeTypeClass) string {
	switch kind.(type) {
	case *tg.AuthCodeTypeSMS:
		return "sms"
	case *tg.AuthCodeTypeCall:
		return "call"
	case *tg.AuthCodeTypeFlashCall:
		return "flash_call"
	case *tg.AuthCodeTypeMissedCall:
		return "missed_call"
	case *tg.AuthCodeTypeFragmentSMS:
		return "fragment_sms"
	default:
		if kind == nil {
			return ""
		}
		return "other"
	}
}

func sentLength(kind tg.AuthSentCodeTypeClass) int {
	switch item := kind.(type) {
	case *tg.AuthSentCodeTypeApp:
		return item.Length
	case *tg.AuthSentCodeTypeSMS:
		return item.Length
	case *tg.AuthSentCodeTypeCall:
		return item.Length
	case *tg.AuthSentCodeTypeMissedCall:
		return item.Length
	case *tg.AuthSentCodeTypeFragmentSMS:
		return item.Length
	case *tg.AuthSentCodeTypeEmailCode:
		return item.Length
	case *tg.AuthSentCodeTypeFirebaseSMS:
		return item.Length
	default:
		return 0
	}
}

func sentEmailPattern(kind tg.AuthSentCodeTypeClass) string {
	item, ok := kind.(*tg.AuthSentCodeTypeEmailCode)
	if !ok {
		return ""
	}
	return item.EmailPattern
}

func sentURL(kind tg.AuthSentCodeTypeClass) string {
	item, ok := kind.(*tg.AuthSentCodeTypeFragmentSMS)
	if !ok {
		return ""
	}
	return item.URL
}

func tlName(v any) string {
	type named interface{ TypeName() string }
	n, ok := v.(named)
	if !ok || n == nil {
		return ""
	}
	return n.TypeName()
}

func sentMessage(status CodeStatus) string {
	switch status.Type {
	case "app":
		return "Enter the code from the Telegram app on your phone."
	case "sms", "sms_word", "sms_phrase":
		return "Enter the code from the text message."
	case "call":
		return "Answer the call and write down the code."
	case "flash_call":
		return "The incoming phone number is the code."
	case "missed_call":
		return "The last digits of the missed call are the code."
	case "fragment_sms":
		if status.URL != "" {
			return "Open the link and read the code there."
		}
		return "Open Fragment and read the code there."
	case "email":
		if status.EmailPattern != "" {
			return "Enter the code sent to " + status.EmailPattern + "."
		}
		return "Enter the code sent to your email."
	case "setup_email":
		return "Enter an email address you can open."
	case "firebase_sms":
		return "Telegram could not text this number. Use the QR code instead."
	case "payment":
		return "Telegram asked for a paid login that this app cannot finish. Use the QR code instead."
	case "already":
		return "This account is already signed in."
	default:
		return "Enter the code Telegram sent you."
	}
}
