package lower

import (
	"strings"
	"testing"
)

// TestImportBuiltinModuleEmitsPackageVar pins where the binding of an ESM import of a
// registry built-in lives. It is a package-level value.Value assigned at the top of
// main, not a local of main: a top-level function that calls the imported module is
// the shape half of Node's test files take, and a Go func cannot see main's locals.
// The assignment sits in main rather than on the var so a module load that throws
// raises inside the program's own error handling instead of at package-init time.
func TestImportBuiltinModuleEmitsPackageVar(t *testing.T) {
	const src = `import assert from 'node:assert';
function check(x: boolean): void { assert.ok(x); }
check(true);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var assert value.Value") {
		t.Errorf("the import binding is not a package-level var:\n%s", out)
	}
	if !strings.Contains(out, `assert = value.RequireBuiltin("node:assert")`) {
		t.Errorf("the module is not loaded at the top of main:\n%s", out)
	}
}

// TestImportBuiltinModuleNamedMemberReadsOffTheModule pins that a named import is a
// member read of the same module value rather than a helper call of its own. The two
// import forms and the require form all reach one module, so a member cannot answer
// differently depending on how the program spelled its import.
func TestImportBuiltinModuleNamedMemberReadsOffTheModule(t *testing.T) {
	const src = `import { strictEqual as eq } from 'node:assert';
eq(1, 1);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, `eq = value.RequireBuiltin("node:assert").Get(value.FromGoString("strictEqual"))`) {
		t.Errorf("an aliased named import did not read its export off the module:\n%s", out)
	}
}

// TestImportBuiltinModuleBareSpecifierEmitsNothing pins the side-effect import. Loading
// a built-in has no effect a program can observe, so the import binds nothing and the
// program emits no load at all.
func TestImportBuiltinModuleBareSpecifierEmitsNothing(t *testing.T) {
	const src = `import 'node:assert';
console.log('loaded');
`
	out := renderProgram(t, src)
	if strings.Contains(out, "RequireBuiltin") {
		t.Errorf("a side-effect import emitted a module load:\n%s", out)
	}
}
