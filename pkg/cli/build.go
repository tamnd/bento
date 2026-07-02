package cli

import (
	"github.com/spf13/cobra"

	"github.com/tamnd/bento/pkg/build"
)

// newBuildCmd is the entry point for the ahead-of-time build path: type-checking
// an entry module, lowering it to Go, and compiling that Go to a single native
// binary. The lowering covers a growing subset of the language; a construct
// outside it fails the build with the compiler's own message rather than
// emitting a program that does not match the source.
func newBuildCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "build <entry>",
		Short: "Compile a TypeScript or JavaScript entry to a single native binary",
		Long: "Build type-checks the entry module, lowers it to Go, and compiles that\n" +
			"Go to a native binary. With no -o the binary takes the entry's base name.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := build.Build(build.Options{Entry: args[0], Output: output}); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "path of the native binary to write (default: entry base name)")
	return cmd
}
