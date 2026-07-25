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
