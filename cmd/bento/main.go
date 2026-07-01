// Command bento is a TypeScript runtime built in Go. It runs TypeScript and
// JavaScript with a pure-Go engine and no cgo, aiming to run existing Node and
// Bun code unchanged while opening the door to any Go library from TypeScript.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tamnd/bento/pkg/cli"
)

func main() {
	// A signal-aware context so Ctrl-C and SIGTERM cancel the running program
	// and let the loop unwind instead of leaving the terminal in a bad state.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.Execute(ctx))
}
