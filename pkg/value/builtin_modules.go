package value

import "strings"

// This file is the runtime side of Node's built-in module registry, the seam the
// whole Node-compatibility roadmap builds against. A Node test reaches a built-in
// with require('assert') or require('node:assert'), both naming the same module, so
// the two specifier forms must resolve to one live value. The registry answers that
// require: it maps every built-in name bento knows to a module value, and each later
// slice replaces a stub entry with a real implementation.
//
// A registered-but-unimplemented built-in returns a throw-on-use stub rather than
// nothing, the honest-stub rule: a test that only requires the module loads and runs
// its body, while a test that actually touches a member of the missing module fails
// with a clear "not implemented" error instead of a silent wrong value or a bare
// "Cannot find module" that would misreport a real Node built-in as absent. The stub
// is a Proxy whose get trap throws, so any property read or method call on it raises
// the error while the module object itself is a live value require hands back.

// builtinModuleNames is the set of Node built-in module names bento's registry
// resolves, keyed by the bare specifier (the node: prefix is stripped before the
// lookup). Every name resolves today to a throw-on-use stub; a later slice swaps an
// entry for a real module without changing this set. The list is the Node 22 built-in
// surface, minus the underscore-prefixed internal modules a program is not meant to
// require by name.
var builtinModuleNames = map[string]bool{
	"assert":              true,
	"assert/strict":       true,
	"async_hooks":         true,
	"buffer":              true,
	"child_process":       true,
	"cluster":             true,
	"console":             true,
	"constants":           true,
	"crypto":              true,
	"dgram":               true,
	"diagnostics_channel": true,
	"dns":                 true,
	"dns/promises":        true,
	"domain":              true,
	"events":              true,
	"fs":                  true,
	"fs/promises":         true,
	"http":                true,
	"http2":               true,
	"https":               true,
	"inspector":           true,
	"inspector/promises":  true,
	"module":              true,
	"net":                 true,
	"os":                  true,
	"path":                true,
	"path/posix":          true,
	"path/win32":          true,
	"perf_hooks":          true,
	"process":             true,
	"punycode":            true,
	"querystring":         true,
	"readline":            true,
	"readline/promises":   true,
	"repl":                true,
	"stream":              true,
	"stream/consumers":    true,
	"stream/promises":     true,
	"stream/web":          true,
	"string_decoder":      true,
	"sys":                 true,
	"timers":              true,
	"timers/promises":     true,
	"tls":                 true,
	"trace_events":        true,
	"tty":                 true,
	"url":                 true,
	"util":                true,
	"util/types":          true,
	"v8":                  true,
	"vm":                  true,
	"wasi":                true,
	"worker_threads":      true,
	"zlib":                true,
}

// builtinModuleCache holds the one module value per canonical built-in name, so a
// second require of the same module (in either specifier form) returns the identical
// value the first require built, the identity require('assert') === require('node:assert')
// the tests rely on. It is a package-level map without a lock, the same single-
// goroutine module-load assumption the rest of the CommonJS runtime makes.
var builtinModuleCache = map[string]Value{}

// canonicalBuiltinName strips the node: scheme from a specifier so require('assert')
// and require('node:assert') land on the same registry key, the interchangeability
// Node gives the two forms for a built-in.
func canonicalBuiltinName(specifier string) string {
	return strings.TrimPrefix(specifier, "node:")
}

// IsBuiltinModule reports whether a require specifier names a Node built-in the
// registry resolves, in either the bare or the node: form. The lowerer calls it to
// decide whether a require('<literal>') lowers to a registry lookup rather than to
// the throwing runtime require, so the built-in name set has one home, here.
func IsBuiltinModule(specifier string) bool {
	return builtinModuleNames[canonicalBuiltinName(specifier)]
}

// RequireBuiltin returns the registered built-in module for a specifier, the value
// require('assert') or require('node:assert') evaluates to. The result is cached by
// canonical name, so the two specifier forms share one identity and a repeated
// require returns the same value. A name outside the registry never reaches here,
// since the lowerer gates the call on IsBuiltinModule; a specifier that slips through
// resolves as a fresh stub rather than panicking, keeping the runtime total.
func RequireBuiltin(specifier string) Value {
	name := canonicalBuiltinName(specifier)
	if m, ok := builtinModuleCache[name]; ok {
		return m
	}
	m := newStubModule(name)
	builtinModuleCache[name] = m
	return m
}

// newStubModule builds the throw-on-use stub for a registered-but-unimplemented
// built-in. It is a Proxy over an empty object whose get trap throws, so requiring
// the module hands back a live value and the module body that only stores it runs,
// while the first read of any member raises a clear error naming the module and the
// member. A later slice replaces the RequireBuiltin entry for a given name with a
// real module, at which point that name no longer reaches this stub.
func newStubModule(name string) Value {
	handler := NewObject()
	handler.Set(FromGoString("get"), NewFunc(func(args []Value) Value {
		member := ToString(Arg(args, 1)).ToGoString()
		Throw(NewError(FromGoString("The built-in module '" + name + "' is registered but not implemented in bento yet (reading '" + member + "')")))
		return Undefined
	}))
	return NewProxy(NewObject(), handler)
}
