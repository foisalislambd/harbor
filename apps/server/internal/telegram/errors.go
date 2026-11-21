package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gotd/td/tgerr"
)

var errLoginClosed = errors.New("telegram login connection closed")

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
	if errors.Is(err, errTelegramUnreachable) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, errLoginClosed) {
		return errTelegramUnreachable
	}
	rpc, ok := tgerr.As(err)
	if !ok {
		return err
	}
	switch rpc.Type {
	case "PHONE_NUMBER_INVALID":
		return errors.New("Telegram rejected that phone number (PHONE_NUMBER_INVALID). Use the international form, such as +15551234567")
	case "PHONE_NUMBER_BANNED":
		return errors.New("Telegram will not send a code to that number (PHONE_NUMBER_BANNED)")
	case "API_ID_INVALID", "API_ID_PUBLISHED_FLOOD":
		return fmt.Errorf("Telegram rejected the api_id or api_hash (%s). Copy both from my.telegram.org/apps for the same application", rpc.Type)
	case "PHONE_CODE_INVALID", "PHONE_CODE_EMPTY", "PHONE_CODE_EXPIRED", "PHONE_CODE_HASH_EMPTY":
		return fmt.Errorf("Telegram rejected the login code (%s). Send a new code and enter the one from the Telegram app", rpc.Type)
	case "PASSWORD_HASH_INVALID":
		return errors.New("Telegram rejected that cloud password (PASSWORD_HASH_INVALID)")
	case "FLOOD_WAIT", "PHONE_NUMBER_FLOOD", "FLOOD_PREMIUM_WAIT":
		if rpc.Argument > 0 {
			return fmt.Errorf("Telegram asked Harbor to wait %d seconds before requesting another code (%s)", rpc.Argument, rpc.Type)
		}
		return fmt.Errorf("Telegram asked Harbor to wait before requesting another code (%s)", rpc.Type)
	default:
		if strings.HasSuffix(rpc.Type, "_MIGRATE") {
			return fmt.Errorf("Telegram asked to switch datacenters (%s) and the switch did not finish", rpc.Message)
		}
		if rpc.Argument > 0 {
			return fmt.Errorf("Telegram error %s (%d)", rpc.Type, rpc.Argument)
		}
		if rpc.Type == "" {
			return err
		}
		return fmt.Errorf("Telegram error %s", rpc.Type)
	}
}
