package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These build real binaries and run them, which is the only place require('assert') is
// checked the way a program experiences it. The messages themselves are unit tested
// against node v24.18.0 in pkg/value over a hundred and forty cases; what that does not prove
// is that the module survives compilation, that the callable module and its strict
// variant come through the lowerer intact, and that a caught AssertionError still
// carries name, code, operator, actual and expected when the program reads them. Every
// expected line here was taken from node v24.18.0 running the same program, except the
// one line TestRequireAssertOkMessageDiverges pins, which says so.

// assertProgram is a program that exercises the module the way a test suite does: the
// calls that must pass silently, then a failure of each shape caught and printed.
//
// The values are literals rather than JSON.parse results because assert takes them as
// arguments and never writes to them, so the fixed-shape limits the deep-equal build
// test works around do not bite here.
const assertProgram = `const assert = require('assert');
console.log(typeof assert, typeof assert.ok, assert.ok === assert, require('node:assert') === assert);
assert(true);
assert.ok(1, 'not used');
assert.equal(1, '1');
assert.deepStrictEqual({ a: 1 }, { a: 1 });
assert.ifError(null);
assert.ifError(undefined);
console.log('passing calls returned');
try {
  assert.strictEqual(1, 2);
} catch (e) {
  console.log(e.name, e.code, e.operator, e.generatedMessage, e.actual, e.expected);
  console.log(e.message);
}
try {
  assert.deepStrictEqual({ a: 1, b: 2 }, { a: 1, b: 3 });
} catch (e) {
  console.log(e.message);
}
try {
  assert.strict.equal(1, '1');
} catch (e) {
  console.log(e.operator);
  console.log(e.message);
}
try {
  assert.ok(false, 'the flag was not set');
} catch (e) {
  console.log(e.message, e.generatedMessage);
}
try {
  assert.ifError(new Error('boom'));
} catch (e) {
  console.log(e.message);
}
try {
  assert.match('bento', /^b.*o$/);
  assert.doesNotMatch('bento', /^x/);
  console.log('match passed');
  assert.match('bento', /^x/);
} catch (e) {
  console.log(e.message);
}
try {
  assert.notDeepStrictEqual([1, 2], [1, 2]);
} catch (e) {
  console.log(e.message);
}
try {
  assert.fail('done');
} catch (e) {
  console.log(e.message, e.code);
}
`

// assertWant is what node v24.18.0 prints for that program. The blank lines are part of
// the messages: an assertion message is a paragraph, not a line, and the diff header
// and the values are separated by one.
const assertWant = `function function true true
passing calls returned
AssertionError ERR_ASSERTION strictEqual true 1 2
Expected values to be strictly equal:

1 !== 2

Expected values to be strictly deep-equal:
+ actual - expected

  {
    a: 1,
+   b: 2
-   b: 3
  }

strictEqual
Expected values to be strictly equal:

1 !== '1'

the flag was not set false
ifError got unwanted exception: boom
match passed
The input did not match the regular expression /^x/. Input:

'bento'

Expected "actual" not to be strictly deep-equal to:

[
  1,
  2
]

done ERR_ASSERTION
`

// TestRequireAssert is the module as a CommonJS program reaches it. The first line is
// the part a require of assert cannot be written without: the module is callable, it is
// its own ok, and both specifier forms name one value.
func TestRequireAssert(t *testing.T) {
	got := buildAndRunFile(t, "main.js", assertProgram)
	if got != assertWant {
		t.Errorf("assert program output\ngot:\n%s\nwant:\n%s", got, assertWant)
	}
}

// TestRequireAssertOkMessageDiverges pins the one message bento cannot match. Node
// generates assert.ok's message by reading the source text of the failed expression off
// disk and quoting it, and a compiled binary has no source to read, so bento prints the
// fallback node itself uses when the source is unavailable. The line below is bento's,
// not node's; node prints "The expression evaluated to a falsy value:\n\n  assert.ok(x)\n".
// A lowering that passed the argument's source text through would close this, and until
// one does the divergence is worth a test of its own rather than a hole in the one above.
func TestRequireAssertOkMessageDiverges(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const assert = require('assert');\n"+
			"const x = false;\n"+
			"try {\n"+
			"  assert.ok(x);\n"+
			"} catch (e) {\n"+
			"  console.log(e.message, e.generatedMessage, e.code);\n"+
			"}\n"+
			"try {\n"+
			"  assert.ok();\n"+
			"} catch (e) {\n"+
			"  console.log(e.message, e.code);\n"+
			"}\n")
	want := "false == true true ERR_ASSERTION\n" +
		"No value argument passed to `assert.ok()` ERR_ASSERTION\n"
	if got != want {
		t.Errorf("assert.ok output\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestRequireAssertUnimplementedMemberFails is the honest-stub rule at the build level.
// rejects is deferred until promises box, and what a program must not get is undefined: a
// read of a member bento does not carry throws, naming the module and the member, so a
// test that uses it fails where it stands rather than several lines later.
func TestRequireAssertUnimplementedMemberFails(t *testing.T) {
	got, err := buildAndRunFileExpectingFailure(t, "main.js",
		"const assert = require('assert');\n"+
			"assert.rejects(function () {}, Error);\n")
	if err == nil {
		t.Fatalf("assert.rejects did not fail, output: %s", got)
	}
	if !strings.Contains(got, "not implemented in bento yet (reading 'rejects')") {
		t.Errorf("assert.rejects error did not name the member: %s", got)
	}
}

// assertThrowsProgram is throws and doesNotThrow as a test suite writes them: a function
// that must raise, matched against each of the four expectations Node accepts, then the
// failure of each shape caught and printed.
//
// The arrow functions are the point as much as the assertions are. An expectation reaches
// the module as a boxed value, and so does the function under test, so this is the path a
// closure, an error constructor named as a value, a regexp literal, an object literal and
// a validation function all take through the lowerer at once.
const assertThrowsProgram = `const assert = require('assert');
assert.throws(() => { throw new TypeError('bad'); }, TypeError);
assert.throws(() => { throw new Error('a'); }, Error);
assert.throws(() => { throw new Error('a'); }, /^Error: a$/);
assert.throws(() => { throw new Error('a'); }, { name: 'Error', message: 'a' });
assert.throws(() => { throw new Error('a'); }, function check(e) { return e.message === 'a'; });
assert.throws(() => { throw new Error('a'); });
assert.doesNotThrow(() => {});
console.log('passing calls returned');
try {
  assert.throws(() => {});
} catch (e) {
  console.log(e.name, e.code, e.operator, e.generatedMessage);
  console.log(e.message);
}
try {
  assert.throws(() => {}, TypeError, 'wanted a throw');
} catch (e) {
  console.log(e.message);
}
try {
  assert.throws(() => { throw new Error('a'); }, TypeError);
} catch (e) {
  console.log(e.message);
}
try {
  assert.throws(() => { throw new TypeError('bad'); }, { name: 'TypeError', message: 'other' });
} catch (e) {
  console.log(e.message);
}
try {
  assert.throws(() => { throw new Error('a'); }, /nope/);
} catch (e) {
  console.log(e.message);
}
try {
  assert.throws(() => { throw new Error('a'); }, function check(e) { return false; });
} catch (e) {
  console.log(e.message);
}
try {
  assert.doesNotThrow(() => { throw new Error('a'); });
} catch (e) {
  console.log(e.name, e.operator);
  console.log(e.message);
}
try {
  assert.doesNotThrow(() => { throw new Error('a'); }, TypeError);
} catch (e) {
  console.log('rethrown', e.name, e.message);
}
try {
  assert.throws(() => { throw new Error('a'); }, 'a');
} catch (e) {
  console.log(e.name, e.code);
}
try {
  assert.throws(42);
} catch (e) {
  console.log(e.code);
}
`

// assertThrowsWant is what node v24.18.0 prints for that program. "Comparison" in the
// diff is node's own placeholder class name, which the message shows because the diff is
// of the keys under comparison rather than of the error itself.
const assertThrowsWant = `passing calls returned
AssertionError ERR_ASSERTION throws false
Missing expected exception.
Missing expected exception (TypeError): wanted a throw
The error is expected to be an instance of "TypeError". Received "Error"

Error message:

a
Expected values to be strictly deep-equal:
+ actual - expected

  Comparison {
+   message: 'bad',
-   message: 'other',
    name: 'TypeError'
  }

The input did not match the regular expression /nope/. Input:

'Error: a'

The "check" validation function is expected to return "true". Received false

Caught error:

Error: a
AssertionError doesNotThrow
Got unwanted exception.
Actual message: "a"
rethrown Error a
TypeError ERR_AMBIGUOUS_ARGUMENT
ERR_INVALID_ARG_TYPE
`

// assertImportProgram is the module reached the ESM way, in every form a program
// binds it: the default import, named imports with one aliased, a namespace import,
// the strict specifier, and the scheme-less specifier. The function is there because
// half of Node's test files call assert from inside one, which a compiled binary can
// only do if the binding is package-level rather than a local of main.
const assertImportProgram = `import assert, { strictEqual, ok as assertOk, throws } from 'node:assert';
import * as ns from 'node:assert';
import strict from 'node:assert/strict';
import bare from 'assert';

function check(x: boolean): void {
  assert.ok(x);
  strictEqual(x, true);
}

check(true);
assertOk(1, 'not used');
ns.deepStrictEqual({ a: 1 }, { a: 1 });
throws(() => { throw new TypeError('bad'); }, TypeError);
console.log('passing calls returned');
console.log(typeof assert, typeof strictEqual, bare === assert, assert.strict === strict);
try {
  strict.equal(1, '1');
} catch (e) {
  console.log(e.operator, e.code);
  console.log(e.message);
}
try {
  assert.strictEqual(1, 2);
} catch (e) {
  console.log(e.message);
}
try {
  throws(() => {});
} catch (e) {
  console.log(e.message);
}
`

// assertImportWant is what node v24.18.0 prints for that program, run as an .mjs with
// the type annotation removed. The identity line is the one worth reading twice: all
// four specifiers name one module value, and the strict module the strict specifier
// loads is the same object assert.strict answers.
const assertImportWant = `passing calls returned
function function true true
strictEqual ERR_ASSERTION
Expected values to be strictly equal:

1 !== '1'

Expected values to be strictly equal:

1 !== 2

Missing expected exception.
`

// TestImportNodeAssert is the import path to the same module the require tests above
// reach. The two must not be able to answer differently, so the assertions here are
// the ones those make, asked through every binding form instead.
func TestImportNodeAssert(t *testing.T) {
	got := buildAndRunFile(t, "main.ts", assertImportProgram)
	if got != assertImportWant {
		t.Errorf("assert import program output\ngot:\n%s\nwant:\n%s", got, assertImportWant)
	}
}

// TestImportNodeAssertBareSpecifier pins the side-effect import. Loading a built-in
// has no effect a program can observe, so the import binds nothing and emits nothing,
// and the program that follows it runs.
func TestImportNodeAssertBareSpecifier(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"import 'node:assert';\n"+
			"console.log('loaded');\n")
	if want := "loaded\n"; got != want {
		t.Errorf("bare import output %q, want %q", got, want)
	}
}

// TestImportNodeAssertUnimplementedMemberFails is the honest-stub rule as the import
// path spells it. A named import binds at load time, so a member bento does not carry
// cannot fail later at its call: it fails the build, naming the member. That is
// stricter than the require path, where the same member throws when it is read, and it
// is the right way round: an import says what it needs before the program runs.
func TestImportNodeAssertUnimplementedMemberFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.ts")
	src := "import { rejects } from 'node:assert';\n" +
		"rejects(function () {}, Error);\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write main.ts: %v", err)
	}
	_, err := Build(Options{Entry: path, Output: filepath.Join(dir, "prog")})
	if err == nil {
		t.Fatal("import of assert.rejects built, want a build error")
	}
	if !strings.Contains(err.Error(), "rejects") {
		t.Errorf("build error did not name the member: %v", err)
	}
}

// TestRequireAssertThrows is assert.throws and assert.doesNotThrow in a compiled binary.
// The last two lines are the argument checking rather than an assertion: a string
// expectation identical to the thrown error's message is refused because such a call
// would assert nothing, and a first argument that is not a function is refused outright.
func TestRequireAssertThrows(t *testing.T) {
	got := buildAndRunFile(t, "main.js", assertThrowsProgram)
	if got != assertThrowsWant {
		t.Errorf("assert.throws program output\ngot:\n%s\nwant:\n%s", got, assertThrowsWant)
	}
}
