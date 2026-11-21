package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gotd/td/tgerr"
)

var errLoginClosed = errors.New("telegram login connection closed")

// errNotSignedIn, errSendCodeFirst, and errLoginLost are client errors.
// They stay 409s, including after the message is redacted.
var (
	errNotSignedIn   = errors.New("sign in to Telegram from Destinations first")
	errSendCodeFirst = errors.New("send a login code first")
	errLoginLost     = errors.New("send a login code again; the Telegram session for this code is gone")
	errQRFirst       = errors.New("show a QR code first")
)

// statusError is a Telegram failure with the HTTP status the panel should get.
// A rejected code or api_id is a 4xx. A datacenter Harbor cannot reach stays a 502.
type statusError struct {
	status int
	text   string
}

func (e *statusError) Error() string {
	if e == nil {
		return ""
	}
	return e.text
}

func (e *statusError) HTTPStatus() int { return e.status }

func statusErr(status int, text string) error {
	return &statusError{status: status, text: text}
}

// StatusCode is the HTTP status for err. Unknown failures are 502.
func StatusCode(err error) int {
	if err == nil {
		return 0
	}
	var httpErr interface{ HTTPStatus() int }
	if errors.As(err, &httpErr) {
		if code := httpErr.HTTPStatus(); code >= 400 {
			return code
		}
	}
	switch {
	case errors.Is(err, errNotSignedIn), errors.Is(err, errSendCodeFirst), errors.Is(err, errLoginLost), errors.Is(err, errQRFirst):
		return 409
	case errors.Is(err, errTelegramUnreachable):
		return 504
	default:
		return 502
	}
}

// errTelegramUnreachable is returned when the datacenter connection does not
// finish inside authBudget. The panel would otherwise wait until a proxy
// such as Cloudflare replaces the response with an HTML error page.
var errTelegramUnreachable = errors.New("timed out reaching Telegram. Allow outbound TCP port 443 from this server to Telegram. A proxy in front of Harbor, such as Cloudflare, must wait longer than this attempt")

func errorsIsTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, errLoginClosed) || errors.Is(err, errTelegramUnreachable)
}

func migrateDC(err error) (int, bool) {
	rpc, ok := tgerr.As(err)
	if !ok || rpc.Argument < 1 || !strings.HasSuffix(rpc.Type, "_MIGRATE") {
		return 0, false
	}
	return rpc.Argument, true
}

func explain(err error) error {
	if err == nil || errors.Is(err, ErrPasswordNeeded) {
		return err
	}
	if errors.Is(err, errLoginClosed) {
		return statusErr(409, "The Telegram login connection closed. Show the QR code or send the login code again.")
	}
	if errors.Is(err, errTelegramUnreachable) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return errTelegramUnreachable
	}
	rpc, ok := tgerr.As(err)
	if !ok {
		return err
	}
	switch rpc.Type {
	case "PHONE_NUMBER_INVALID":
		return statusErr(400, "Telegram rejected that phone number (PHONE_NUMBER_INVALID). Use the international form, such as +15551234567")
	case "PHONE_NUMBER_BANNED":
		return statusErr(400, "Telegram will not send a code to that number (PHONE_NUMBER_BANNED)")
	case "API_ID_INVALID", "API_ID_PUBLISHED_FLOOD":
		return statusErr(400, fmt.Sprintf("Telegram rejected the api_id or api_hash (%s). Copy both from my.telegram.org/apps for the same application", rpc.Type))
	case "PHONE_CODE_INVALID", "PHONE_CODE_EMPTY", "PHONE_CODE_EXPIRED", "PHONE_CODE_HASH_EMPTY":
		return statusErr(400, fmt.Sprintf("Telegram rejected the login code (%s). Send a new code and enter the one from the delivery shown in the panel", rpc.Type))
	case "PASSWORD_HASH_INVALID":
		return statusErr(400, "Telegram rejected that cloud password (PASSWORD_HASH_INVALID)")
	case "FLOOD_WAIT", "PHONE_NUMBER_FLOOD", "FLOOD_PREMIUM_WAIT":
		if rpc.Argument > 0 {
			return statusErr(429, fmt.Sprintf("Telegram asked Harbor to wait %d seconds before requesting another code (%s)", rpc.Argument, rpc.Type))
		}
		return statusErr(429, fmt.Sprintf("Telegram asked Harbor to wait before requesting another code (%s)", rpc.Type))
	default:
		if strings.HasSuffix(rpc.Type, "_MIGRATE") {
			name := rpc.Message
			if name == "" {
				name = rpc.Type
			}
			return statusErr(502, fmt.Sprintf("Telegram asked to switch datacenters (%s) and the switch did not finish", name))
		}
		status := 502
		switch rpc.Code {
		case 400, 406:
			status = 400
		case 401:
			status = 401
		case 420:
			status = 429
		}
		if rpc.Argument > 0 {
			return statusErr(status, fmt.Sprintf("Telegram error %s (%d)", rpc.Type, rpc.Argument))
		}
		if rpc.Type == "" {
			return err
		}
		return statusErr(status, fmt.Sprintf("Telegram error %s", rpc.Type))
	}
}
