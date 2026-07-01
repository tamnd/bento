package cli

// Build metadata, stamped via -ldflags at release time. goreleaser and the
// Makefile target github.com/tamnd/bento/pkg/cli.{Version,Commit,Date}.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
