// Package engine defines the JavaScript engine service provider interface that
// the rest of bento runs on top of.
//
// bento does not hard-wire a single JS engine. The runtime talks to whatever
// backend implements Engine, and backends register themselves by name at init
// time. The default build ships the pure-Go quickjs backend so there is no cgo
// and the binary cross-compiles everywhere. Other backends (goja, a wasm tier,
// or cgo v8/jsc) can be added behind build tags and selected at startup.
//
// An Engine owns one JavaScript realm and is not safe for concurrent use. The
// event loop drives it from a single goroutine, which matches JavaScript's
// run-to-completion, single-threaded execution model.
package engine

import (
	"fmt"
	"sort"
	"sync"
)

// HostFunc is a Go function exposed to JavaScript as a global callable. The
// arguments arrive already converted to native Go values and the return value
// is converted back to a JavaScript value. Returning an error surfaces as a
// thrown JavaScript exception.
type HostFunc func(args []any) (any, error)

// ModuleLoader resolves a module specifier to its source text. It is used by
// engines that support native ES modules. Returning an error rejects the import.
type ModuleLoader func(specifier string) (string, error)

// Engine is a JavaScript realm that bento can evaluate code in.
//
// Method names follow the shape of the underlying quickjs API so backends stay
// thin, but the interface is deliberately engine neutral so goja, wasm, or a
// cgo engine can satisfy it too.
type Engine interface {
	// Name reports the backend identifier, for example "quickjs".
	Name() string

	// Eval runs source as global (script) code and returns the completion
	// value converted to a native Go value. The name is used for diagnostics
	// only and may be ignored by backends that do not track script names.
	Eval(name, source string) (any, error)

	// EvalModule runs source as ES module code. Imports are resolved through
	// the loader set by SetModuleLoader.
	EvalModule(name, source string) error

	// Call invokes a global function by name with the given native Go
	// arguments and returns its converted result.
	Call(fn string, args ...any) (any, error)

	// Register installs fn as a global function named name. It is how bento
	// wires host capabilities such as printing, timers, and process control
	// into the JavaScript world.
	Register(name string, fn HostFunc) error

	// DrainMicrotasks runs the pending job queue (resolved promises and
	// queued microtasks) to completion and reports how many jobs ran.
	DrainMicrotasks() (int, error)

	// SetModuleLoader installs the resolver used by EvalModule and by native
	// import statements.
	SetModuleLoader(loader ModuleLoader)

	// Interrupt requests that a running evaluation stop as soon as possible.
	// It is safe to call from another goroutine.
	Interrupt()

	// Close releases the underlying realm. The Engine must not be used after.
	Close() error
}

// Factory builds a fresh Engine instance.
type Factory func() (Engine, error)

var (
	mu       sync.RWMutex
	backends = map[string]Factory{}
	// defaultName is the backend used when the caller does not ask for a
	// specific one. The first backend to register claims it, and the quickjs
	// backend registers in the default build, so quickjs is the default.
	defaultName string
)

// Register adds a backend under name. Backends call this from an init function.
// The first registration also becomes the default backend.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := backends[name]; dup {
		panic(fmt.Sprintf("engine: backend %q registered twice", name))
	}
	backends[name] = f
	if defaultName == "" {
		defaultName = name
	}
}

// SetDefault chooses which registered backend New uses when asked for the empty
// name. It returns an error if the backend is not registered.
func SetDefault(name string) error {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := backends[name]; !ok {
		return fmt.Errorf("engine: no backend named %q (have %v)", name, sortedNames())
	}
	defaultName = name
	return nil
}

// Default reports the name of the default backend, or the empty string if none
// is registered.
func Default() string {
	mu.RLock()
	defer mu.RUnlock()
	return defaultName
}

// Available lists the registered backend names in sorted order.
func Available() []string {
	mu.RLock()
	defer mu.RUnlock()
	return sortedNames()
}

// New builds an Engine for the named backend. An empty name selects the default
// backend.
func New(name string) (Engine, error) {
	mu.RLock()
	if name == "" {
		name = defaultName
	}
	f, ok := backends[name]
	have := sortedNames()
	mu.RUnlock()

	if name == "" {
		return nil, fmt.Errorf("engine: no backends registered")
	}
	if !ok {
		return nil, fmt.Errorf("engine: no backend named %q (have %v)", name, have)
	}
	return f()
}

func sortedNames() []string {
	names := make([]string, 0, len(backends))
	for n := range backends {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
