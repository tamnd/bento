package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newTestCmd will run a project's test files with the built-in test runner. The
// runner (bento:test, describe/it, expect) is specified for a later milestone,
// so this reserves the command and says so plainly.
func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test [patterns...]",
		Short: "Run tests with the built-in runner (coming soon)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return errComingSoon("test", "the built-in test runner")
		},
	}
	return cmd
}

// errComingSoon is the shared message for commands whose milestone has not
// landed yet. It names what is missing so the output is honest.
func errComingSoon(cmd, what string) error {
	return fmt.Errorf("bento %s: %s is not implemented in this build yet", cmd, what)
}
