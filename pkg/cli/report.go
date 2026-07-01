package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/tamnd/bento/pkg/engine"
)

// errSilent is returned after we have already printed a program's uncaught
// exception ourselves. Execute still exits non-zero, but fang does not render it
// a second time in its generic error box.
var errSilent = errors.New("")

// reportUncaught inspects an error from running a program. An uncaught
// JavaScript exception is printed to stderr the way a JS developer expects and
// converted to errSilent so the CLI exits 1 without fang reformatting it.
// Everything else (a missing file, a bad flag) passes through to fang unchanged.
func reportUncaught(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	var ex *engine.Exception
	if errors.As(err, &ex) {
		cmd.PrintErrln(ex.Display())
		return errSilent
	}
	return err
}
