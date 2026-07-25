package build

import "testing"

// TestRequireBuiltinResolvesBothForms pins slice G1.1: require('assert') and
// require('node:assert') resolve through the built-in registry to the same live
// value. typeof the module is "object" because requiring an unimplemented built-in
// hands back a real value rather than throwing, and the two specifier forms share
// one identity, the interchangeability Node gives the bare and node: forms. Node
// prints "object" then "true".
func TestRequireBuiltinResolvesBothForms(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = require('assert');\n"+
			"const b = require('node:assert');\n"+
			"console.log(typeof a);\n"+
			"console.log(a === b);\n")
	if want := "object\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestModuleBuiltinReadsRegistry pins the last G1.1 clause: the module core module
// reflects the built-in registry back to the program. require('node:module') hands
// back a real module, not a stub, so isBuiltin answers over the registered name set in
// either specifier form, and builtinModules is a live array of the registered names.
// isBuiltin reads the same set require resolves on, so it is true for a real built-in
// in the bare or the node: form and false for a relative path. Node prints "true",
// "true", "false", "true", then "true".
func TestModuleBuiltinReadsRegistry(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const m = require('node:module');\n"+
			"console.log(m.isBuiltin('fs'));\n"+
			"console.log(m.isBuiltin('node:fs'));\n"+
			"console.log(m.isBuiltin('./local'));\n"+
			"console.log(Array.isArray(m.builtinModules));\n"+
			"console.log(m.builtinModules.length > 0);\n")
	if want := "true\ntrue\nfalse\ntrue\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestModuleBuiltinBareAndNodeFormShareIdentity pins that the module core module
// itself obeys the registry identity rule: require('module') and require('node:module')
// are the one cached value, so the bare and node: forms are interchangeable for the
// registry's own reflection just as they are for any other built-in. Node prints "true".
func TestModuleBuiltinBareAndNodeFormShareIdentity(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = require('module');\n"+
			"const b = require('node:module');\n"+
			"console.log(a === b);\n")
	if want := "true\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireBuiltinStubThrowsOnUse pins the honest-stub rule: requiring an
// unimplemented built-in loads, but touching a member throws a clear error naming
// the module and the member rather than resolving to a silent wrong value. The body
// requires the module without incident, then a member read inside a try surfaces the
// message. Node has a real assert, so this behavior is bento-specific and pinned by
// its own message rather than compared against Node.
func TestRequireBuiltinStubThrowsOnUse(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = require('assert');\n"+
			"console.log('loaded');\n"+
			"try { a.ok(true); } catch (e) { console.log(e.message); }\n")
	want := "loaded\n" +
		"The built-in module 'assert' is registered but not implemented in bento yet (reading 'ok')\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
