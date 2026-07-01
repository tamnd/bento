// Package runtime wires an engine and an event loop into a working JavaScript
// host and runs a program.
//
// It installs the primitive host functions the prelude builds on (writing to the
// console, scheduling timers, reading boot data, exiting the process), evaluates
// the prelude to construct the global environment, transpiles and runs the entry
// file, and then pumps the event loop until the program is done.
package runtime

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/tamnd/bento/pkg/engine"
	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/loop"
	"github.com/tamnd/bento/pkg/node"
)

//go:embed prelude.js
var preludeSource string

// nodeCompatVersion is the Node.js version bento reports through process.version.
// Existing packages branch on this, and the spec pins Node 22 as the baseline.
const nodeCompatVersion = "22.11.0"

// Config controls how a program runs.
type Config struct {
	// Argv is the full argument vector. Index 0 is the executable path, index 1
	// is the entry script, and the rest are user arguments, matching Node.
	Argv []string
	// EngineName selects the engine backend. Empty uses the default (quickjs).
	EngineName string
	// BentoVersion is reported through process.versions.bento.
	BentoVersion string
	// Stdout and Stderr default to os.Stdout and os.Stderr when nil.
	Stdout io.Writer
	Stderr io.Writer
}

// Runtime is a live JavaScript host bound to one engine and loop.
type Runtime struct {
	cfg     Config
	eng     engine.Engine
	loop    *loop.Loop
	stdout  io.Writer
	stderr  io.Writer
	started time.Time
}

// New builds a runtime and its global environment. Close must be called when
// done.
func New(cfg Config) (*Runtime, error) {
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}

	eng, err := engine.New(cfg.EngineName)
	if err != nil {
		return nil, err
	}

	rt := &Runtime{
		cfg:     cfg,
		eng:     eng,
		loop:    loop.New(eng),
		stdout:  cfg.Stdout,
		stderr:  cfg.Stderr,
		started: time.Now(),
	}

	if err := rt.installHostFuncs(); err != nil {
		_ = eng.Close()
		return nil, err
	}
	if _, err := eng.Eval("<prelude>", preludeSource); err != nil {
		_ = eng.Close()
		return nil, fmt.Errorf("bento: prelude failed: %w", err)
	}
	if err := node.Install(eng); err != nil {
		_ = eng.Close()
		return nil, fmt.Errorf("bento: node layer failed: %w", err)
	}
	return rt, nil
}

// Engine exposes the underlying engine for the node layer and higher milestones.
func (rt *Runtime) Engine() engine.Engine { return rt.eng }

// Loop exposes the event loop so I/O modules can register handles.
func (rt *Runtime) Loop() *loop.Loop { return rt.loop }

// Close releases the engine.
func (rt *Runtime) Close() error { return rt.eng.Close() }

// RunFile transpiles and runs the entry file, then pumps the event loop until
// the program settles.
func (rt *Runtime) RunFile(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("bento: read %s: %w", path, err)
	}
	res, err := frontend.Transpile(string(src), frontend.Options{Filename: path})
	if err != nil {
		return err
	}
	if err := rt.evalEntry(path, res.Code); err != nil {
		return err
	}
	return rt.loop.Run()
}

// RunString transpiles and runs source as if it were the named entry file. It is
// used by the REPL and by eval flags.
func (rt *Runtime) RunString(name, source string) error {
	res, err := frontend.Transpile(source, frontend.Options{Filename: name})
	if err != nil {
		return err
	}
	if err := rt.evalEntry(name, res.Code); err != nil {
		return err
	}
	return rt.loop.Run()
}

// evalEntry runs the entry module's transpiled code in the global scope. The
// prelude already provides module, exports, and require, so CommonJS output runs
// directly.
func (rt *Runtime) evalEntry(path, code string) error {
	if _, err := rt.eng.Eval(filepath.Base(path), code); err != nil {
		return err
	}
	return nil
}

// installHostFuncs registers the primitive bridge the prelude builds on.
func (rt *Runtime) installHostFuncs() error {
	reg := func(name string, fn engine.HostFunc) error { return rt.eng.Register(name, fn) }

	if err := reg("__bento_write", func(args []any) (any, error) {
		fd := toInt(arg(args, 0))
		s := toString(arg(args, 1))
		w := rt.stdout
		if fd == 2 {
			w = rt.stderr
		}
		_, _ = io.WriteString(w, s)
		return nil, nil
	}); err != nil {
		return err
	}

	if err := reg("__bento_setTimer", func(args []any) (any, error) {
		rt.loop.AddTimer(toInt64(arg(args, 0)), toInt(arg(args, 1)), toBool(arg(args, 2)))
		return nil, nil
	}); err != nil {
		return err
	}

	if err := reg("__bento_clearTimer", func(args []any) (any, error) {
		rt.loop.ClearTimer(toInt64(arg(args, 0)))
		return nil, nil
	}); err != nil {
		return err
	}

	if err := reg("__bento_boot", func(args []any) (any, error) {
		return rt.bootJSON()
	}); err != nil {
		return err
	}

	if err := reg("__bento_cwd", func(args []any) (any, error) {
		wd, err := os.Getwd()
		if err != nil {
			return "", nil
		}
		return wd, nil
	}); err != nil {
		return err
	}

	if err := reg("__bento_now", func(args []any) (any, error) {
		return float64(time.Since(rt.started).Nanoseconds()) / 1e6, nil
	}); err != nil {
		return err
	}

	if err := reg("__bento_hrtime", func(args []any) (any, error) {
		return float64(time.Since(rt.started).Nanoseconds()), nil
	}); err != nil {
		return err
	}

	if err := reg("__bento_exit", func(args []any) (any, error) {
		code := toInt(arg(args, 0))
		if f, ok := rt.stdout.(interface{ Sync() error }); ok {
			_ = f.Sync()
		}
		os.Exit(code)
		return nil, nil
	}); err != nil {
		return err
	}

	return nil
}

// bootJSON serializes the process environment the prelude reads once at startup.
func (rt *Runtime) bootJSON() (string, error) {
	argv := rt.cfg.Argv
	if len(argv) == 0 {
		argv = []string{"bento"}
	}
	exec, _ := os.Executable()
	if exec == "" {
		exec = argv[0]
	}

	boot := map[string]any{
		"argv":     argv,
		"argv0":    argv[0],
		"execPath": exec,
		"env":      envMap(),
		"platform": nodePlatform(),
		"arch":     nodeArch(),
		"pid":      os.Getpid(),
		"version":  "v" + nodeCompatVersion,
		"versions": map[string]string{
			"node":  nodeCompatVersion,
			"bento": rt.cfg.BentoVersion,
			"v8":    "12.4.0",
		},
	}
	b, err := json.Marshal(boot)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func envMap() map[string]string {
	out := map[string]string{}
	for _, kv := range os.Environ() {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				out[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	return out
}

// nodePlatform maps Go's GOOS to the strings Node uses for process.platform.
func nodePlatform() string {
	switch runtime.GOOS {
	case "windows":
		return "win32"
	default:
		return runtime.GOOS
	}
}

// nodeArch maps Go's GOARCH to the strings Node uses for process.arch.
func nodeArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "ia32"
	default:
		return runtime.GOARCH
	}
}
