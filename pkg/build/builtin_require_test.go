package build

import "testing"

// TestRequireBuiltinResolvesBothForms pins slice G1.1: require('assert') and
// require('node:assert') resolve through the built-in registry to the same live
// value, the interchangeability Node gives the bare and node: forms. typeof the
// module is "function" because assert is callable, assert(value) being the same
// assertion as assert.ok(value); it read "object" while assert was a stub. Node
// prints "function" then "true".
func TestRequireBuiltinResolvesBothForms(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = require('assert');\n"+
			"const b = require('node:assert');\n"+
			"console.log(typeof a);\n"+
			"console.log(a === b);\n")
	if want := "function\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireUnimplementedBuiltinIsAnObject pins the other half of that: a built-in
// bento has not implemented still requires to a live value rather than throwing at
// the require, so a module body that only stores it runs. fs stands in for the
// registered-but-unimplemented set here, which is where assert was until it became a
// real module. Node prints "object" then "true" for this too, since its fs is an
// object as well.
func TestRequireUnimplementedBuiltinIsAnObject(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = require('fs');\n"+
			"const b = require('node:fs');\n"+
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
// message. Node has a real fs, so this behavior is bento-specific and pinned by its
// own message rather than compared against Node. It used to read assert, which is a
// real module now.
func TestRequireBuiltinStubThrowsOnUse(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = require('fs');\n"+
			"console.log('loaded');\n"+
			"try { a.readFileSync('x'); } catch (e) { console.log(e.message); }\n")
	want := "loaded\n" +
		"The built-in module 'fs' is registered but not implemented in bento yet (reading 'readFileSync')\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
