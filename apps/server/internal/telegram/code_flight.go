package telegram

import (
	"context"
	"log"
	"sync"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

// codeFlight is one auth.sendCode for an api id and phone. A second request
// for the same account waits for it instead of creating another auth key.
// Two keys invalidate each other's phone_code_hash, which is what made
// sign-in return PHONE_CODE_EXPIRED a moment later.
type codeFlight struct {
	done       chan struct{}
	status     CodeStatus
	err        error
	session    []byte
	hash       string
	user       string
	authorized bool
	premium    bool
	peer       *storedPeer
}

type codeFlights struct {
	mu    sync.Mutex
	items map[string]*codeFlight
}

func newCodeFlights() *codeFlights {
	return &codeFlights{items: map[string]*codeFlight{}}
}

// codeSends is process-local, like the login connection it protects.
var codeSends = newCodeFlights()

func (h *codeFlights) begin(key string) (*codeFlight, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing := h.items[key]; existing != nil {
		return existing, false
	}
	flight := &codeFlight{done: make(chan struct{})}
	h.items[key] = flight
	return flight, true
}

func (h *codeFlights) finish(key string, flight *codeFlight, cred *credentials) {
	flight.session = append([]byte(nil), cred.Session...)
	flight.hash = cred.PhoneCodeHash
	flight.user = cred.User
	flight.authorized = cred.Authorized
	flight.premium = cred.Premium
	if cred.Peer != nil {
		peer := *cred.Peer
		flight.peer = &peer
	}
	h.mu.Lock()
	if h.items[key] == flight {
		delete(h.items, key)
	}
	h.mu.Unlock()
	close(flight.done)
}

func (f *codeFlight) apply(cred *credentials) {
	cred.Session = append([]byte(nil), f.session...)
	cred.PhoneCodeHash = f.hash
	cred.User = f.user
	cred.Authorized = f.authorized
	cred.Premium = f.premium
	if f.peer != nil {
		peer := *f.peer
		cred.Peer = &peer
	} else {
		cred.Peer = nil
	}
	cred.persist = true
}

func (g gotdEngine) SendCode(ctx context.Context, apiID int, cred *credentials, phone string) (CodeStatus, error) {
	key := loginKey(apiID, phone, cred.APIHash)
	flight, leader := codeSends.begin(key)
	if !leader {
		log.Printf("telegram auth.sendCode joined an in-flight request")
		select {
		case <-ctx.Done():
			return CodeStatus{}, redact(explain(ctx.Err()), *cred)
		case <-flight.done:
			flight.apply(cred)
			return flight.status, flight.err
		}
	}
	// Only the leader replaces the auth key. Joined callers reuse its result
	// and must not save an empty session over it if they give up first.
	cred.persist = true
	defer codeSends.finish(key, flight, cred)
	if cred.Authorized {
		if err := g.LogOut(ctx, apiID, cred, phone); err != nil {
			flight.err = err
			return CodeStatus{}, err
		}
	}
	cred.Session = nil
	cred.PhoneCodeHash = ""
	cred.Authorized = false
	cred.User = ""
	cred.Peer = nil
	status, err := g.exchangeCode(ctx, apiID, phone, cred, true, func(ctx context.Context, client *telegram.Client) (tg.AuthSentCodeClass, error) {
		return sendCodeOnDC(ctx, client, phone)
	})
	flight.status = status
	flight.err = err
	return status, err
}
