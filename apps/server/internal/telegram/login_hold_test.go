package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tgerr"
)

func TestLoginHoldReusesOneConnection(t *testing.T) {
	hub := newLoginHub()
	var connections int
	hold := hub.start("acct", nil, nil, func(ctx context.Context, storage *memorySession, serve func(context.Context, *telegram.Client) error) error {
		connections++
		storage.data = []byte("key")
		return serve(ctx, nil)
	})
	calls := 0
	call := func(context.Context, *telegram.Client) error {
		calls++
		return nil
	}
	if err := hold.do(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	if err := hold.do(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	if connections != 1 || calls != 2 || string(hold.storage.snapshot()) != "key" {
		t.Fatalf("connections %d calls %d session %q", connections, calls, hold.storage.snapshot())
	}
	hub.drop("acct")
	if hub.get("acct") != nil {
		t.Fatal("hold still registered")
	}
}

func TestLoginHoldStopsAStuckConnect(t *testing.T) {
	hub := newLoginHub()
	started := make(chan struct{})
	hold := hub.start("acct", nil, nil, func(ctx context.Context, _ *memorySession, _ func(context.Context, *telegram.Client) error) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := hold.do(ctx, func(context.Context, *telegram.Client) error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	hub.drop("acct")
	if hub.get("acct") != nil {
		t.Fatal("stuck hold still registered")
	}
}

func TestExplainTelegramErrors(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{tgerr.New(400, "PHONE_NUMBER_INVALID"), "PHONE_NUMBER_INVALID"},
		{tgerr.New(400, "API_ID_INVALID"), "API_ID_INVALID"},
		{tgerr.New(420, "FLOOD_WAIT_32"), "32 seconds"},
		{tgerr.New(303, "PHONE_MIGRATE_2"), "PHONE_MIGRATE_2"},
		{context.DeadlineExceeded, "timed out reaching Telegram"},
	}
	for _, tc := range cases {
		got := explain(tc.err)
		if got == nil || !strings.Contains(got.Error(), tc.want) {
			t.Fatalf("%v -> %v", tc.err, got)
		}
	}
	dc, ok := migrateDC(tgerr.New(303, "PHONE_MIGRATE_2"))
	if !ok || dc != 2 {
		t.Fatalf("migrate %d %v", dc, ok)
	}
	if _, ok := migrateDC(tgerr.New(400, "PHONE_NUMBER_INVALID")); ok {
		t.Fatal("not a migration")
	}
	if !errors.Is(explain(ErrPasswordNeeded), ErrPasswordNeeded) {
		t.Fatal("password error was rewritten")
	}
	if StatusCode(explain(tgerr.New(400, "PHONE_CODE_EXPIRED"))) != 400 {
		t.Fatal("expired code should be a client error")
	}
	if StatusCode(explain(tgerr.New(420, "FLOOD_WAIT_32"))) != 429 {
		t.Fatal("flood wait should be 429")
	}
	if StatusCode(errNotSignedIn) != 409 || StatusCode(errTelegramUnreachable) != 504 {
		t.Fatal("local status")
	}
	redacted := redact(explain(tgerr.New(400, "PHONE_CODE_EXPIRED")), credentials{})
	if StatusCode(redacted) != 400 {
		t.Fatalf("redact dropped status: %v", StatusCode(redacted))
	}
}

func TestSendCodeFlightJoins(t *testing.T) {
	key := "join-test"
	leader, lead := codeSends.begin(key)
	if !lead {
		t.Fatal("expected the leader")
	}
	started := make(chan struct{})
	joined := make(chan *codeFlight, 1)
	go func() {
		flight, ok := codeSends.begin(key)
		if ok {
			t.Error("second call became a leader")
		}
		close(started)
		<-flight.done
		joined <- flight
	}()
	<-started
	leader.status = CodeStatus{Type: "app", TypeName: "auth.sentCodeTypeApp"}
	cred := &credentials{Session: []byte("same-key"), PhoneCodeHash: "hash", persist: true}
	codeSends.finish(key, leader, cred)
	flight := <-joined
	other := &credentials{}
	flight.apply(other)
	if string(other.Session) != "same-key" || other.PhoneCodeHash != "hash" || !other.persist {
		t.Fatalf("%+v", other)
	}
	if again, lead := codeSends.begin(key); !lead {
		t.Fatal("flight was not cleared")
	} else {
		codeSends.finish(key, again, &credentials{})
	}
}
