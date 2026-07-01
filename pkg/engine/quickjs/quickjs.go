// Package quickjs is the default bento JavaScript engine backend. It wraps
// modernc.org/quickjs, a pure-Go build of the QuickJS ES2023 engine, so bento
// runs JavaScript with zero cgo and cross-compiles to every supported platform.
//
// The backend registers itself as "quickjs" in the default build, which makes
// it the default engine. It is not safe for concurrent use; the event loop
// drives it from one goroutine.
package quickjs

import (
	"fmt"
	"strings"

	"github.com/tamnd/bento/pkg/engine"
	qjs "modernc.org/quickjs"
)

func init() {
	engine.Register("quickjs", func() (engine.Engine, error) { return newEngine() })
}

// Engine implements engine.Engine on top of a single quickjs realm.
type Engine struct {
	vm     *qjs.VM
	loader engine.ModuleLoader
	host   engine.ModuleHost
}

func newEngine() (*Engine, error) {
	vm, err := qjs.NewVM()
	if err != nil {
		return nil, fmt.Errorf("quickjs: create realm: %w", err)
	}
	// QuickJS defaults to disallowing blocking; the event loop never blocks
	// the realm itself, but Atomics and similar expect this enabled.
	vm.SetCanBlock(true)
	return &Engine{vm: vm}, nil
}

// Name reports the backend identifier.
func (e *Engine) Name() string { return "quickjs" }

// Eval runs source as global script code.
func (e *Engine) Eval(name, source string) (any, error) {
	r, err := e.vm.Eval(source, qjs.EvalGlobal)
	if err != nil {
		return nil, wrap(name, err)
	}
	return r, nil
}

// EvalModule runs source as ES module code.
func (e *Engine) EvalModule(name, source string) error {
	if _, err := e.vm.Eval(source, qjs.EvalModule); err != nil {
		return wrap(name, err)
	}
	return nil
}

// Call invokes a global function by name.
func (e *Engine) Call(fn string, args ...any) (any, error) {
	r, err := e.vm.Call(fn, args...)
	if err != nil {
		return nil, wrap(fn, err)
	}
	return r, nil
}

// Register installs a Go function as a global callable.
func (e *Engine) Register(name string, fn engine.HostFunc) error {
	if err := e.vm.RegisterHostFunc(name, qjs.HostFunc(fn)); err != nil {
		return fmt.Errorf("quickjs: register %s: %w", name, err)
	}
	return nil
}

// DrainMicrotasks runs the pending promise and microtask jobs to completion.
func (e *Engine) DrainMicrotasks() (int, error) {
	n, err := e.vm.ExecutePendingJobs()
	if err != nil {
		return n, fmt.Errorf("quickjs: run microtasks: %w", err)
	}
	return n, nil
}

// SetModuleLoader installs the source-only resolver used for native module
// imports. It relies on quickjs's default specifier normalization.
func (e *Engine) SetModuleLoader(loader engine.ModuleLoader) {
	e.loader = loader
	if loader == nil {
		e.vm.SetModuleLoader(nil, nil)
		return
	}
	e.vm.SetModuleLoader(
		func(_ *qjs.VM, name string) (string, error) { return loader(name) },
		nil,
	)
}

// SetModuleHost installs a resolver that both normalizes specifiers and loads
// module source, wiring quickjs's custom normalize and load callbacks. It is how
// the runtime routes native ES imports through the bento resolver.
func (e *Engine) SetModuleHost(host engine.ModuleHost) {
	e.host = host
	if host == nil {
		e.vm.SetModuleLoader(nil, nil)
		return
	}
	e.vm.SetModuleLoader(
		func(_ *qjs.VM, name string) (string, error) { return host.Load(name) },
		func(_ *qjs.VM, base, name string) (string, error) { return host.Normalize(base, name) },
	)
}

// Interrupt asks the running evaluation to stop. It is safe to call from
// another goroutine.
func (e *Engine) Interrupt() { e.vm.Interrupt() }

// Close releases the realm.
func (e *Engine) Close() error { return e.vm.Close() }

// wrap turns a quickjs evaluation error into an engine.Exception so callers can
// recognize an uncaught JavaScript throw and render it the way a JS developer
// expects, while keeping the original error reachable through errors.Unwrap.
func wrap(name string, err error) error {
	return &engine.Exception{
		Message: strings.TrimSpace(err.Error()),
		Script:  name,
	}
}
