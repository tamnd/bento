package frontend

import (
	"errors"

	"github.com/tamnd/bento/pkg/frontend/adapter"
)

// ErrRealAdapterUnavailable reports that no real typescript-go-backed checker
// can be constructed in this build. typescript-go keeps its checker, binder, and
// parser under internal/ as of mid-2026, so there is no public API to drive and
// no revision to pin (see pkg/frontend/adapter/version.go). The partitioner and
// lowering are developed against adapter.NewFake until the upstream API lands in
// TypeScript 7.1 or a bento fork exposes it through a public shim; at that point
// Load builds a real Program and this error goes away.
var ErrRealAdapterUnavailable = errors.New(
	"frontend: real typescript-go adapter unavailable (upstream API is internal until TS 7.1); " +
		"use frontend.Wrap with a supplied adapter for now")

// FileSystem replaces the real file system for a load. A nil FileSystem reads
// through the OS. Tests and the bundler feed virtual modules through it.
type FileSystem interface {
	ReadFile(path string) (string, bool)
	FileExists(path string) bool
	DirectoryExists(path string) bool
}

// LoadOptions controls how a typed program is loaded. It mirrors the parts of a
// tsconfig bento honors, plus bento-specific knobs. It is named LoadOptions
// rather than Options because the transpile path already owns frontend.Options.
type LoadOptions struct {
	// Dir is the project root; tsconfig discovery starts here.
	Dir string
	// ConfigPath, if set, names a tsconfig explicitly and skips discovery.
	ConfigPath string
	// Roots are entry files; if empty they come from the tsconfig include set.
	Roots []string
	// Overrides are compiler-option overrides applied on top of the tsconfig.
	Overrides ConfigOverrides
	// FS, if non-nil, replaces the real file system.
	FS FileSystem
}

// ConfigOverrides are compiler-option overrides applied on top of the resolved
// tsconfig. Zero-valued fields leave the tsconfig value in place.
type ConfigOverrides struct {
	Strict       *bool
	SkipLibCheck *bool
	AllowJS      *bool
	CheckJS      *bool
	Paths        map[string][]string
}

// Load discovers or reads the tsconfig, constructs the compiler host, builds the
// typescript-go program, runs the binder, and wraps the result in a bento
// Program. It does not force a full type check; the checker runs lazily on first
// query.
//
// It returns ErrRealAdapterUnavailable while the real adapter is blocked
// upstream. The wiring above it (tsconfig resolution, host construction, root
// discovery) is stable and lands in follow-up slices; the one missing piece is
// the adapter.TSAdapter implementation, which is why the failure is a single
// clear error rather than a panic.
func Load(opts LoadOptions) (*Program, error) {
	if !adapter.RealAdapterAvailable() {
		return nil, ErrRealAdapterUnavailable
	}
	// When a real adapter exists, this path resolves the tsconfig into
	// CompilerOptions and a root set, builds the host over opts.FS or the OS,
	// calls adapter.BuildProgram, and returns Wrap(realAdapter, handle).
	return nil, ErrRealAdapterUnavailable
}
