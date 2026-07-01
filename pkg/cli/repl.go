package cli

import (
	"bufio"
	"fmt"
	"io"
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
			defer rt.Close()

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "bento %s repl, engine %s. Ctrl-D to exit.\n", Version, rt.Engine().Name())

			in := bufio.NewScanner(cmd.InOrStdin())
			for {
				fmt.Fprint(out, "> ")
				if !in.Scan() {
					fmt.Fprintln(out)
					return nil
				}
				line := strings.TrimSpace(in.Text())
				if line == "" {
					continue
				}
				if line == ".exit" {
					return nil
				}
				evalLine(out, rt, line)
			}
		},
	}
	cmd.Flags().StringVar(&engineName, "engine", "", "JavaScript engine backend (default quickjs)")
	return cmd
}

func evalLine(out io.Writer, rt *runtime.Runtime, line string) {
	res, err := frontend.Transpile(line, frontend.Options{Filename: "repl.ts"})
	if err != nil {
		fmt.Fprintln(out, err)
		return
	}
	v, err := rt.Engine().Eval("repl", res.Code)
	if err != nil {
		fmt.Fprintln(out, err)
		return
	}
	if _, err := rt.Engine().DrainMicrotasks(); err != nil {
		fmt.Fprintln(out, err)
		return
	}
	if v != nil {
		fmt.Fprintf(out, "%v\n", v)
	}
}
