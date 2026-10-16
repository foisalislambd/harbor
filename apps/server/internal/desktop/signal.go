package desktop

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// WaitInterrupt blocks until the process is asked to stop.
func WaitInterrupt() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
