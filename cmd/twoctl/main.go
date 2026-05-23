package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/two-inc/twoctl-cli/cmd/twoctl/cli"
)

func main() {
	// SIGINT / SIGTERM cancel the root context so any in-flight HTTP
	// request can abort cleanly instead of hanging until the client
	// timeout fires. Without this, Ctrl-C from the user only takes
	// effect after the 60s client timeout - which forces a kill -9 in
	// practice.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cli.HandleError(cli.Root().ExecuteContext(ctx))
}
