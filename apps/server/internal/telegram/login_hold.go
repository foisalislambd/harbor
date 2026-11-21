package telegram

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/gotd/td/telegram"
)

const (
	// authBudget stays under Cloudflare's 100 second proxy timeout.
	authBudget = 45 * time.Second
	// loginKeepAlive keeps the MTProto connection used for auth.sendCode open
	// until the code or the cloud password is submitted.
	loginKeepAlive = 3 * time.Minute
)

type holdCall struct {
	ctx  context.Context
	fn   func(context.Context, *telegram.Client) error
	done chan error
}

type loginHold struct {
	storage *memorySession
	calls   chan holdCall
	cancel  context.CancelFunc
	closed  chan struct{}
}

type loginRunner func(ctx context.Context, storage *memorySession, serve func(context.Context, *telegram.Client) error) error

type loginHub struct {
	mu    sync.Mutex
	items map[string]*loginHold
}

func newLoginHub() *loginHub {
	return &loginHub{items: map[string]*loginHold{}}
}

// authSessions is process-local. A restart falls back to the saved session.
var authSessions = newLoginHub()

func loginKey(apiID int, phone, hash string) string {
	return strconv.Itoa(apiID) + "\n" + phone + "\n" + hash
}

func (h *loginHub) get(key string) *loginHold {
	h.mu.Lock()
	defer h.mu.Unlock()
	item := h.items[key]
	if item == nil {
		return nil
	}
	select {
	case <-item.closed:
		delete(h.items, key)
		return nil
	default:
		return item
	}
}

func (h *loginHub) drop(key string) {
	h.mu.Lock()
	item := h.items[key]
	delete(h.items, key)
	h.mu.Unlock()
	if item == nil {
		return
	}
	item.cancel()
	<-item.closed
}

func (h *loginHub) start(key string, session []byte, run loginRunner) *loginHold {
	h.drop(key)
	ctx, cancel := context.WithTimeout(context.Background(), loginKeepAlive)
	item := &loginHold{
		storage: &memorySession{data: append([]byte(nil), session...)},
		calls:   make(chan holdCall),
		cancel:  cancel,
		closed:  make(chan struct{}),
	}
	h.mu.Lock()
	h.items[key] = item
	h.mu.Unlock()
	go func() {
		defer close(item.closed)
		defer cancel()
		_ = run(ctx, item.storage, item.serve)
	}()
	return item
}

func (s *loginHold) serve(ctx context.Context, client *telegram.Client) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case call := <-s.calls:
			err := call.fn(call.ctx, client)
			select {
			case call.done <- err:
			default:
			}
		}
	}
}

func (s *loginHold) do(ctx context.Context, fn func(context.Context, *telegram.Client) error) error {
	done := make(chan error, 1)
	call := holdCall{ctx: ctx, fn: fn, done: done}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closed:
		return errLoginClosed
	case s.calls <- call:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closed:
		return errLoginClosed
	case err := <-done:
		return err
	}
}

func (g gotdEngine) authCall(ctx context.Context, apiID int, phone string, cred *credentials, fresh bool, fn func(context.Context, *telegram.Client) error) error {
	key := loginKey(apiID, phone, cred.APIHash)
	hold := authSessions.get(key)
	if hold == nil || fresh {
		hold = authSessions.start(key, cred.Session, func(ctx context.Context, storage *memorySession, serve func(context.Context, *telegram.Client) error) error {
			return g.runStored(ctx, apiID, cred.APIHash, storage, true, func(ctx context.Context, client *telegram.Client) error {
				return serve(ctx, client)
			})
		})
	}
	err := hold.do(ctx, fn)
	if snap := hold.storage.snapshot(); len(snap) > 0 {
		cred.Session = snap
	}
	if errorsIsTimeout(err) {
		authSessions.drop(key)
	}
	if errors.Is(err, ErrPasswordNeeded) {
		return ErrPasswordNeeded
	}
	return redact(explain(err), *cred)
}
