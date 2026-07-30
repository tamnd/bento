package build

import (
	"strings"
	"testing"
)

// These build real binaries and run them, which is the only place require('assert') is
// checked the way a program experiences it. The messages themselves are unit tested
// against node v24.18.0 in pkg/value over eighty-seven cases; what that does not prove
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
// throws is deferred to its own slice, and what a program must not get is undefined: a
// read of a member bento does not carry throws, naming the module and the member, so a
// test that uses it fails where it stands rather than several lines later.
func TestRequireAssertUnimplementedMemberFails(t *testing.T) {
	got, err := buildAndRunFileExpectingFailure(t, "main.js",
		"const assert = require('assert');\n"+
			"assert.throws(function () {}, Error);\n")
	if err == nil {
		t.Fatalf("assert.throws did not fail, output: %s", got)
	}
	if !strings.Contains(got, "not implemented in bento yet (reading 'throws')") {
		t.Errorf("assert.throws error did not name the member: %s", got)
	}
}
