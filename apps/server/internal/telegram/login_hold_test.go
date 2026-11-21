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
	hold := hub.start("acct", nil, func(ctx context.Context, storage *memorySession, serve func(context.Context, *telegram.Client) error) error {
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
	hold := hub.start("acct", nil, func(ctx context.Context, _ *memorySession, _ func(context.Context, *telegram.Client) error) error {
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
}
