package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatecannotbealtered/gitlab-cli/cmd"
)

var osExit = os.Exit

func main() {
	osExit(run(os.Args))
}

func run(args []string) int {
	if len(args) > 0 {
		os.Args = args
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, cmd.ErrSilent) {
			return cmd.LastExitCode()
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		return cmd.ExitBadArgs
	}
	return 0
}
