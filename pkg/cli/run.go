package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tamnd/bento/pkg/runtime"
)

// newRunCmd runs a TypeScript or JavaScript entry file. Arguments after the file
// are passed through to the program as process.argv, matching node and bun.
func newRunCmd() *cobra.Command {
	var engineName string

	cmd := &cobra.Command{
		Use:   "run <file> [args...]",
		Short: "Run a TypeScript or JavaScript file",
		Long: "Run transpiles the entry file and executes it, then pumps the event loop\n" +
			"until the program settles. Arguments after the file are forwarded to the\n" +
			"program through process.argv.",
		Args:                  cobra.MinimumNArgs(1),
		DisableFlagsInUseLine: true,
		// Stop parsing flags after the entry file so the program gets its own
		// arguments verbatim, even when they look like bento flags.
		FParseErrWhitelist: cobra.FParseErrWhitelist{},
		RunE: func(cmd *cobra.Command, args []string) error {
			entry := args[0]
			abs, err := filepath.Abs(entry)
			if err != nil {
				abs = entry
			}
			if _, err := os.Stat(abs); err != nil {
				return fmt.Errorf("bento run: %s: %w", entry, err)
			}

			self, _ := os.Executable()
			if self == "" {
				self = "bento"
			}
			argv := append([]string{self, abs}, args[1:]...)

			rt, err := runtime.New(runtime.Config{
				Argv:         argv,
				EngineName:   engineName,
				BentoVersion: Version,
				Stdout:       cmd.OutOrStdout(),
				Stderr:       cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			defer func() { _ = rt.Close() }()

			return reportUncaught(cmd, rt.RunFile(abs))
		},
	}

	cmd.Flags().StringVar(&engineName, "engine", "", "JavaScript engine backend (default quickjs)")
	// Treat everything after the entry file as program arguments.
	cmd.Flags().SetInterspersed(false)
	return cmd
}
