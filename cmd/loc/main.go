// cmd/loc/main.go

// Command loc compiles one canonical locale source into backend and frontend locale files.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/smegg99/s99term"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// stderr is the human stream and stdout the machine one, so `loc check --json` pipes cleanly.
	a := newApp(os.Stderr, os.Stdout, os.Environ())
	// os.Exit skips a deferred call, so the two exit paths close it themselves.
	defer a.close()

	if err := rootCmd(a).ExecuteContext(ctx); err != nil {
		// Exit 1: the check failed and the command printed why. Exit 2: loc could not run.
		if errors.Is(err, errFailed) {
			a.close()
			os.Exit(1)
		}
		a.warns(s99term.StatusFailed, err.Error())
		a.close()
		os.Exit(2)
	}
}
