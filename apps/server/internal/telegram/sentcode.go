package telegram

import (
	"errors"
	"fmt"
	"log"
	"strings"

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
	var text string
	switch status.Type {
	case "app":
		text = "Telegram sent the code to your other logged-in devices, in the service chat named Telegram (777000). This Harbor session does not receive it. Open Telegram on a phone that is already signed in to this number."
	case "sms":
		text = "Telegram sent the code by SMS."
	case "call":
		text = "Telegram is calling this number and will read the code."
	case "flash_call":
		text = "Telegram will flash-call this number. The calling phone number is the code."
	case "missed_call":
		text = "Telegram will call and hang up. The last digits of that phone number are the code."
	case "fragment_sms":
		text = "Telegram sent the code through Fragment."
		if status.URL != "" {
			text += " Open " + status.URL + ", sign in with the wallet for this number, and read the code there."
		} else {
			text += " Sign in at fragment.com with the wallet for this number and read the code there."
		}
	case "email":
		if status.EmailPattern != "" {
			text = "Telegram sent the login code to the login email " + status.EmailPattern + "."
		} else {
			text = "Telegram sent the login code to the login email on this account."
		}
	case "setup_email":
		text = "Telegram will not send a login code until this number has a login email. Enter an email you can open. Telegram sends a confirmation code there, and then the login code."
	case "firebase_sms":
		text = "Telegram asked for a Firebase SMS check that only official mobile apps can finish. Resend the code to use the next method."
	case "sms_word":
		text = "Telegram sent an SMS containing one word. That word is the code."
	case "sms_phrase":
		text = "Telegram sent an SMS containing a phrase. That phrase is the code."
	default:
		text = "Telegram answered auth.sentCode with type " + status.Type + "."
	}
	if status.Length > 0 && status.Type != "setup_email" && status.Type != "flash_call" {
		text += fmt.Sprintf(" It is %d characters.", status.Length)
	}
	if status.NextType != "" {
		wait := ""
		if status.Timeout > 0 {
			wait = fmt.Sprintf(" If it does not arrive within %d seconds, resend it", status.Timeout)
		} else {
			wait = " If it does not arrive, resend it"
		}
		text += wait + " by " + strings.ReplaceAll(status.NextType, "_", " ") + "."
	}
	if status.TypeName != "" {
		text = status.TypeName + ". " + text
	}
	return text
}
