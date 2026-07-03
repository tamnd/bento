package frontend

import (
	"errors"
	"os"

	"github.com/tamnd/bento/pkg/frontend/adapter"
	"github.com/tamnd/bento/pkg/resolve"
)

// ErrRealAdapterUnavailable reports that no real typescript-go-backed checker can
// be constructed in this build. It is a defensive guard only: bento consumes
// typescript-go through the tamnd/typescript fork, whose public shim exposes the
// checker, so the real adapter is always available and RealAdapterAvailable
// returns true (see pkg/frontend/adapter/version.go). The error remains so a
// build that somehow dropped the pin fails loudly rather than nil-crashing.
var ErrRealAdapterUnavailable = errors.New(
	"frontend: real typescript-go adapter unavailable (no fork revision pinned); " +
		"use frontend.Wrap with a supplied adapter")

// ErrNoRoots reports that Load was called with no entry files. tsconfig include
// discovery, which fills the root set from the project when none is given, is a
// later slice; until then a caller names its roots explicitly.
var ErrNoRoots = errors.New(
	"frontend: Load requires at least one root file (tsconfig include discovery is a later slice)")

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
	if len(opts.Roots) == 0 {
		return nil, ErrNoRoots
	}

	fs := opts.FS
	if fs == nil {
		fs = osFileSystem{}
	}

	cwd := opts.Dir
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		} else {
			cwd = "/"
		}
	}

	// The host reads through an overlay that adds bento's ambient Node
	// declarations, while the resolver keeps the raw FS so its osFileSystem fast
	// path (real directory listings for node_modules) is unaffected. The ambient
	// file is prepended to the roots so the checker parses it and its declarations
	// become globals across the program.
	// The host reads through two overlays: the ambient one adds bento's Node and
	// bento:go declarations, and the Go one serves a generated .d.ts for each go:
	// import at the virtual path it resolves to. The resolver keeps the raw FS so
	// its osFileSystem fast path is unaffected.
	host := &loadHost{
		fs:       newGoDeclOverlay(ambientOverlay{base: fs}, goDeclCache()),
		resolver: resolve.New(resolve.Options{FS: fsForResolver(fs), Dev: true}),
		cwd:      cwd,
	}

	co := adapter.CompilerOptions{Strict: true}
	applyOverrides(&co, opts.Overrides)

	roots := append([]string{ambientPath}, opts.Roots...)

	a := adapter.NewReal()
	h, err := a.BuildProgram(roots, co, host)
	if err != nil {
		return nil, err
	}
	return Wrap(a, h), nil
}

// applyOverrides folds the caller's compiler-option overrides onto the base
// options. A nil override pointer leaves the base value in place, so a caller
// changes only what it names.
func applyOverrides(co *adapter.CompilerOptions, ov ConfigOverrides) {
	if ov.Strict != nil {
		co.Strict = *ov.Strict
	}
	if ov.SkipLibCheck != nil {
		co.SkipLibCheck = *ov.SkipLibCheck
	}
	if ov.AllowJS != nil {
		co.AllowJS = *ov.AllowJS
	}
	if ov.CheckJS != nil {
		co.CheckJS = *ov.CheckJS
	}
	if ov.Paths != nil {
		co.Paths = ov.Paths
	}
}
