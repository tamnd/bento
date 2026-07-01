package cli

import (
	"github.com/spf13/cobra"
)

// newBuildCmd is the entry point for the ahead-of-time build path: bundling and
// compiling a project down to a single native binary. The compiler pipeline
// (partitioner, type lowering, Go codegen) lands in later milestones, so for now
// this reserves the command and reports that clearly rather than pretending.
func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build <entry>",
		Short: "Compile a project to a single binary (coming soon)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return errComingSoon("build", "the ahead-of-time compiler")
		},
	}
	return cmd
}
