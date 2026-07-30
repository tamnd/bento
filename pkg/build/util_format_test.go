package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These build real binaries and run them, which is the only place the util.format
// slice is checked the way a program experiences it. The port itself is unit tested
// against node v24.18.0 in pkg/value, and the routing is unit tested in pkg/lower;
// what neither proves is that a console.log with a specifier in a compiled binary
// prints what node prints, that require('util') survives compilation, and that the
// import form and the require form answer the same. Every expected string here was
// taken from node v24.18.0 running the same program.

// TestConsoleLogFormatsSpecifiers is the line the slice exists for: console.log with
// a format string used to print the specifier itself and then the arguments after it,
// so "%s is %d" came out with the percent signs still in it.
func TestConsoleLogFormatsSpecifiers(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log('%s is %d', 'x', 2);\n"+
			"console.log('%i and %f', '42px', '3.5rem');\n"+
			"console.log('%j', { a: 1 });\n"+
			"console.log('%O', { a: 1 });\n"+
			"console.log('%s', 'a', 'b', 'c');\n"+
			"console.log('%d%s', 1);\n"+
			"console.log('%s %s', true, null);\n"+
			"console.error('%d', 1.5);\n"+
			"console.log('%s', [1, 2, 3]);\n"+
			"console.log('%s', 'x %s y', 'z');\n")
	want := "x is 2\n" +
		"42 and 3.5\n" +
		`{"a":1}` + "\n" +
		"{ a: 1 }\n" +
		"a b c\n" +
		"1%s\n" +
		"true null\n" +
		"1.5\n" +
		"[ 1, 2, 3 ]\n" +
		"x %s y z\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireUtilIsARealModule pins the module a compiled CommonJS program gets.
// require('util') used to be the throw-on-use stub, so the first member read failed.
// The identity line is the registry rule: both specifier forms name one module.
func TestRequireUtilIsARealModule(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const util = require('util');\n"+
			"console.log(util.format('%s|%d', 'a', 1));\n"+
			"console.log(util.inspect({ a: 1, b: 'x' }));\n"+
			"console.log(util.inspect({ a: { b: { c: 1 } } }, { depth: 0 }));\n"+
			"console.log(util.formatWithOptions({ numericSeparator: true }, '%d', 1234567));\n"+
			"console.log(util === require('node:util'));\n"+
			"console.log(typeof util.format);\n"+
			"console.log(util.format('no specifiers here'));\n"+
			"console.log(util.format('%s', 1, { z: 2 }));\n")
	want := "a|1\n" +
		"{ a: 1, b: 'x' }\n" +
		"{ a: [Object] }\n" +
		"1_234_567\n" +
		"true\n" +
		"function\n" +
		"no specifiers here\n" +
		"1 { z: 2 }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestImportNodeUtilAnswersLikeRequire pins the other half of the module. An import of
// node:util lowers to direct calls into the value helpers and a require of it reads the
// registry module, two different paths through the compiler that must not be able to
// print two different things.
func TestImportNodeUtilAnswersLikeRequire(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"import { format, inspect, formatWithOptions } from 'node:util';\n"+
			"import util from 'node:util';\n"+
			"console.log(format('%s|%d', 'a', 1));\n"+
			"console.log(inspect({ a: 1 }));\n"+
			"console.log(inspect({ a: { b: 1 } }, { depth: 0 }));\n"+
			"console.log(formatWithOptions({ numericSeparator: true }, '%d', 1234567));\n"+
			"console.log(util.format('%s', 'via the module object'));\n"+
			"console.log(format() === '');\n")
	want := "a|1\n" +
		"{ a: 1 }\n" +
		"{ a: [Object] }\n" +
		"1_234_567\n" +
		"via the module object\n" +
		"true\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAMissingUtilMemberFailsAtTheRead pins the honest-stub rule from inside a
// compiled binary. util.promisify is not implemented, and a program that reads it
// stops there with an error naming the member, rather than getting undefined and
// failing one line later on a call to it.
func TestAMissingUtilMemberFailsAtTheRead(t *testing.T) {
	out, err := buildAndRunFileExpectingFailure(t, "main.js",
		"const util = require('util');\n"+
			"console.log('before');\n"+
			"const p = util.promisify;\n"+
			"console.log('after');\n")
	if err == nil {
		t.Fatalf("the program exited zero, want a failure: %q", out)
	}
	if !strings.Contains(out, "before\n") {
		t.Errorf("the program failed before its first line: %q", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("the read of util.promisify did not stop the program: %q", out)
	}
	if !strings.Contains(out, "not implemented in bento yet (reading 'promisify')") {
		t.Errorf("the error did not name the member: %q", out)
	}
}

// buildAndRunFileExpectingFailure is buildAndRunFile for a program meant to fail at
// run time: it still requires the build to succeed, and returns the output and the
// run error rather than failing the test on a nonzero exit.
func buildAndRunFileExpectingFailure(t *testing.T, name, src string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	bin := filepath.Join(dir, "prog")
	prog, err := Build(Options{Entry: path, Output: bin})
	if err != nil {
		t.Fatalf("build %s: %v", name, err)
	}
	out, runErr := exec.Command(prog).CombinedOutput()
	return string(out), runErr
}
