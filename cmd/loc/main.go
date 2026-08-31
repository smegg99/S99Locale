// cmd/loc/main.go

// loc: one canonical locale source, compiled into per-target bundles.
//
// The backend wants flat keys and {{.Name}}; the frontend wants nested keys,
// {name} and pipe-separated plural bands. Writing both by hand is how a string
// drifts and how a Polish count silently renders the wrong case.
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
