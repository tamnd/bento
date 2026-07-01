// Package cli is bento's command surface: the cobra tree, the global flags, and
// the fang-rendered help and errors. The runtime work lives under pkg/runtime
// and the engine packages; this layer only parses arguments and hands off.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	// Register the default engine backend. Importing it for its side effect is
	// what makes quickjs available and the default. Swapping engines later is a
	// matter of importing a different backend under a build tag.
	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

// Execute builds the root command and runs it through fang. main passes the
// signal-aware context so Ctrl-C cancels the running program. It returns the
// process exit code.
func Execute(ctx context.Context) int {
	root := newRoot()
	opts := []fang.Option{
		fang.WithVersion(Version),
		// Uncaught program exceptions are already printed Node-style by the run
		// and repl commands, which return errSilent. Skip fang's error box for
		// those and render everything else the normal way.
		fang.WithErrorHandler(func(w io.Writer, styles fang.Styles, err error) {
			if errors.Is(err, errSilent) {
				return
			}
			fang.DefaultErrorHandler(w, styles, err)
		}),
	}
	if err := fang.Execute(ctx, root, opts...); err != nil {
		return 1
	}
	return 0
}

// newRoot assembles the command tree.
func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "bento",
		Short: "A TypeScript runtime built in Go",
		Long: "bento (弁当) runs TypeScript and JavaScript with a pure-Go engine and no\n" +
			"cgo. It aims to run existing Node and Bun code unchanged, and it lets you\n" +
			"reach into any Go library from TypeScript. This build ships the core run\n" +
			"path; more of the surface lands milestone by milestone.",
		Version:       fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newRunCmd())
	root.AddCommand(newReplCmd())
	root.AddCommand(newBuildCmd())
	root.AddCommand(newTestCmd())
	root.AddCommand(newVersionCmd())
	return root
}

// newVersionCmd prints the version triple. fang also wires --version on the
// root, but a plain subcommand is handy in scripts.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the bento version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Printf("bento %s (commit %s, built %s)\n", Version, Commit, Date)
			return nil
		},
	}
}
