package cli

import (
	"bufio"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/runtime"
)

// newReplCmd starts an interactive read-eval-print loop. Each line is transpiled
// and evaluated in one long-lived realm, then pending promises are drained and
// the completion value is printed.
func newReplCmd() *cobra.Command {
	var engineName string

	cmd := &cobra.Command{
		Use:   "repl",
		Short: "Start an interactive TypeScript prompt",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := runtime.New(runtime.Config{
				Argv:         []string{"bento", "repl"},
				EngineName:   engineName,
				BentoVersion: Version,
				Stdout:       cmd.OutOrStdout(),
				Stderr:       cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			defer func() { _ = rt.Close() }()

			cmd.Printf("bento %s repl, engine %s. Ctrl-D to exit.\n", Version, rt.Engine().Name())

			in := bufio.NewScanner(cmd.InOrStdin())
			for {
				cmd.Print("> ")
				if !in.Scan() {
					cmd.Println()
					return nil
				}
				line := strings.TrimSpace(in.Text())
				if line == "" {
					continue
				}
				if line == ".exit" {
					return nil
				}
				evalLine(cmd, rt, line)
			}
		},
	}
	cmd.Flags().StringVar(&engineName, "engine", "", "JavaScript engine backend (default quickjs)")
	return cmd
}

// evalLine transpiles and runs one REPL line, printing either the error or the
// completion value. Errors are shown and swallowed so the session continues.
func evalLine(cmd *cobra.Command, rt *runtime.Runtime, line string) {
	res, err := frontend.Transpile(line, frontend.Options{Filename: "repl.ts"})
	if err != nil {
		cmd.Println(err)
		return
	}
	v, err := rt.Engine().Eval("repl", res.Code)
	if err != nil {
		cmd.Println(err)
		return
	}
	if _, err := rt.Engine().DrainMicrotasks(); err != nil {
		cmd.Println(err)
		return
	}
	if v != nil {
		cmd.Printf("%v\n", v)
	}
}
