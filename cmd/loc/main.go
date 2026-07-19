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
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// stderr is the human stream and stdout the machine one, so `loc check --json` pipes cleanly.
	a := newApp(os.Stderr, os.Stdout)

	if err := rootCmd(a).ExecuteContext(ctx); err != nil {
		// Exit 1: the check failed and the command printed why. Exit 2: loc could not run.
		if errors.Is(err, errFailed) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "loc:", err)
		os.Exit(2)
	}
}
