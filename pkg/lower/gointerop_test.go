package lower

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/goimport"
)

// testGoSignatures is the signature resolver the program tests wire into the
// renderer, loading each Go package's signatures once and memoizing them, so a go:
// call marshals numbers by the real Go type the same way the build does. It is the
// test-side twin of build.goSignatureResolver.
func testGoSignatures() func(importPath, name string) (goimport.FuncSig, bool) {
	memo := map[string]map[string]goimport.FuncSig{}
	return func(importPath, name string) (goimport.FuncSig, bool) {
		sigs, loaded := memo[importPath]
		if !loaded {
			var err error
			sigs, err = goimport.Signatures(importPath)
			if err != nil {
				sigs = map[string]goimport.FuncSig{}
			}
			memo[importPath] = sigs
		}
		sig, ok := sigs[name]
		return sig, ok
	}
}

// This file covers the go: import lowering: a call to a name a go: import binds
// lowers to a direct call into the real Go package, with the value crossings run
// through the interop bridge. The unit cases pin the pieces that need no toolchain
// (the alias picker, the path parser), and the end-to-end case compiles and runs a
// module that calls into the standard library so the generated import, the call
// qualifier, and the bridge marshaling are all proven against a real build.

// TestGoImportPathStripsSchemeAndVersion checks the import path a go: specifier
// lowers to, with and without a pinned version, since the emitted import is by
// path and the version rides only the specifier.
func TestGoImportPathStripsSchemeAndVersion(t *testing.T) {
	cases := map[string]string{
		"go:strings":       "strings",
		"go:crypto/sha256": "crypto/sha256",
		"go:github.com/klauspost/compress/zstd@v1.17": "github.com/klauspost/compress/zstd",
	}
	for module, want := range cases {
		if got := goImportPath(module); got != want {
			t.Errorf("goImportPath(%q) = %q, want %q", module, got, want)
		}
	}
}

// TestGoAliasBaseIsTheLastSegment checks the alias a package imports under is its
// last path segment sanitized to a Go identifier, the form the spec's examples use
// and the one that matches the package's own name for the common case.
func TestGoAliasBaseIsTheLastSegment(t *testing.T) {
	cases := map[string]string{
		"strings":                            "strings",
		"crypto/sha256":                      "sha256",
		"github.com/klauspost/compress/zstd": "zstd",
		"gopkg.in/yaml.v2":                   "yaml_v2",
	}
	for path, want := range cases {
		if got := goAliasBase(path); got != want {
			t.Errorf("goAliasBase(%q) = %q, want %q", path, got, want)
		}
	}
}

// TestGoAliasIsUniqueAcrossPackages proves two packages that share a last segment
// get distinct aliases, so the emitted imports never collide.
func TestGoAliasIsUniqueAcrossPackages(t *testing.T) {
	r := NewRenderer(nil)
	first := r.requireGoImport("crypto/sha256")
	second := r.requireGoImport("other/sha256")
	if first == second {
		t.Fatalf("two sha256 packages share alias %q", first)
	}
	if first != "sha256" {
		t.Errorf("first sha256 alias = %q, want sha256", first)
	}
	// The same path asked twice keeps the alias it was already assigned.
	if again := r.requireGoImport("crypto/sha256"); again != first {
		t.Errorf("re-request of crypto/sha256 alias = %q, want %q", again, first)
	}
}

// TestGoAliasAvoidsRuntimeNames proves an import path whose last segment is a
// reserved runtime name (value, bridge) is aliased away from it, so the go: import
// never shadows the value model or the interop bridge in the emitted file.
func TestGoAliasAvoidsRuntimeNames(t *testing.T) {
	r := NewRenderer(nil)
	if got := r.requireGoImport("example.com/value"); got == "value" {
		t.Errorf("go: package aliased to value, shadowing the value model")
	}
	if got := r.requireGoImport("example.com/bridge"); got == "bridge" {
		t.Errorf("go: package aliased to bridge, shadowing the interop bridge")
	}
}

// TestGoImportProgramRuns proves the go: lowering end to end: a module that calls
// strings.ToUpper (string in, string out) and strings.HasPrefix (strings in, bool
// out) compiles to a program that imports the standard library package under an
// alias and marshals through the bridge, and running it prints the values the
// TypeScript would. The standard library is offline and always present, so this is
// a sound oracle without a network fetch.
func TestGoImportProgramRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: interop test builds and runs generated Go")
	}
	const src = `import { ToUpper, HasPrefix } from "go:strings";
const shout = ToUpper("hello");
const ok = HasPrefix(shout, "HE");
console.log(shout);
console.log(ok);
`
	got := runProgramGo(t, src)
	if want := "HELLO\ntrue\n"; got != want {
		t.Fatalf("go: interop program printed %q, want %q", got, want)
	}
}

// TestGoImportMarshalsNumbers proves the signature-driven number crossings end to
// end: strconv.Itoa marshals a number argument to a Go int, math.Abs crosses a
// float64 both ways with no conversion, and utf8.RuneCountInString returns a Go int
// widened back to a number through the range check. Running the binary is the
// oracle, since the whole point is that the marshaling the signature drives is the
// one the Go toolchain compiles and runs.
func TestGoImportMarshalsNumbers(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: number test builds and runs generated Go")
	}
	const src = `import { Itoa } from "go:strconv";
import { Abs } from "go:math";
import { RuneCountInString } from "go:unicode/utf8";
console.log(Itoa(42));
console.log(Abs(-3.5));
console.log(RuneCountInString("héllo"));
`
	got := runProgramGo(t, src)
	if want := "42\n3.5\n5\n"; got != want {
		t.Fatalf("go: number program printed %q, want %q", got, want)
	}
}

// TestGoImportNumberNeedsSignature proves the honest fallback: with no signature
// resolver wired, a go: call whose crossing needs the Go type (a number argument or
// result) hands back rather than guess, so the unit routes to the engine. The
// string and boolean crossings still lower, because the TypeScript type settles
// them without the Go signature.
func TestGoImportNumberNeedsSignature(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Itoa } from "go:strconv";
console.log(Itoa(42));
`
	prog := compile(t, src)
	r := NewRenderer(prog) // no SetGoSignatures: the type-only path must hand a number back
	if _, err := r.RenderProgram(entryFile(t, prog)); err == nil {
		t.Fatal("a number crossing lowered with no signature resolver, want a hand-back")
	}
}

// TestGoImportErrorHoistsToThrow pins the (T, error) throw bridge shape: a call to
// strconv.Atoi, whose Go signature is (int, error), lowers to the Go call wrapped
// in bridge.Must so the error hoists to a throw and the int result crosses back
// through the range check, the mapping section 6.6 fixes.
func TestGoImportErrorHoistsToThrow(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Atoi } from "go:strconv";
console.log(Atoi("42"));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "bridge.Must(strconv.Atoi(") {
		t.Errorf("a (T, error) call did not wrap the Go call in the throw bridge:\n%s", source)
	}
	if !strings.Contains(source, "bridge.Int64ToNumber(int64(bridge.Must(") {
		t.Errorf("the throw bridge's int result did not cross back through the range check:\n%s", source)
	}
	if !strings.Contains(source, "defer value.ReportUncaught()") {
		t.Errorf("a program that can raise a go: error did not defer the uncaught reporter:\n%s", source)
	}
}

// TestGoImportErrorBridgeRuns proves the throw bridge end to end: strconv.Atoi on a
// valid number returns the parsed value, and on a bad number the returned Go error
// hoists to a throw that a bento catch recovers and reads as an Error whose message
// is the Go error string, the round trip section 7.7 promises.
func TestGoImportErrorBridgeRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: error test builds and runs generated Go")
	}
	const src = `import { Atoi } from "go:strconv";
console.log(Atoi("42"));
try {
  Atoi("nope");
} catch (e) {
  if (e instanceof Error) {
    console.log("caught: " + e.message);
  }
}
`
	got := runProgramGo(t, src)
	if want := "42\ncaught: strconv.Atoi: parsing \"nope\": invalid syntax\n"; got != want {
		t.Fatalf("go: error bridge program printed %q, want %q", got, want)
	}
}

// TestGoImportNamespaceRuns proves the namespace form end to end: a module that
// imports a Go package as a namespace (import * as strs) and calls its members
// through the binding compiles to the same direct Go calls a named import would,
// so strs.ToUpper and strs.HasPrefix cross through the bridge and a second
// namespace's sc.Itoa marshals a number, all in one program (section 3).
func TestGoImportNamespaceRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the namespace go: test builds and runs generated Go")
	}
	const src = `import * as strs from "go:strings";
import * as sc from "go:strconv";
const shout = strs.ToUpper("hello");
console.log(shout);
console.log(strs.HasPrefix(shout, "HE"));
console.log(sc.Itoa(42));
`
	got := runProgramGo(t, src)
	if want := "HELLO\ntrue\n42\n"; got != want {
		t.Fatalf("namespace go: program printed %q, want %q", got, want)
	}
}

// TestGoImportNamespaceEmitsDirectCall pins that a namespace member call lowers to
// the qualified Go call through the bridge, the same shape a named import emits, so
// the binding is purely a call-site convenience and adds no indirection.
func TestGoImportNamespaceEmitsDirectCall(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import * as strs from "go:strings";
console.log(strs.ToUpper("hi"));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "strings.ToUpper(bridge.StringToGo(") {
		t.Errorf("a namespace member call did not lower to the direct Go call through the bridge:\n%s", source)
	}
}

// TestGoImportWrapsCallInBoundaryGuard pins that a go: call lowers through the
// boundary guard of section 12.3: the marshaled result rides inside a bridge.Guard
// closure that returns the bento result type, so a panic from the Go library
// converts to a catchable thrown GoError, and the program defers the uncaught
// reporter because a guarded call can raise.
func TestGoImportWrapsCallInBoundaryGuard(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { ToUpper } from "go:strings";
console.log(ToUpper("hi"));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "bridge.Guard(func() value.BStr") {
		t.Errorf("a string go: result was not wrapped in the boundary guard:\n%s", source)
	}
	if !strings.Contains(source, "bridge.StringFromGo(strings.ToUpper(bridge.StringToGo(") {
		t.Errorf("the guarded call did not keep the direct bridge crossing:\n%s", source)
	}
	if !strings.Contains(source, "defer value.ReportUncaught()") {
		t.Errorf("a program with a guarded go: call did not defer the uncaught reporter:\n%s", source)
	}
}

// TestGoImportGuardsVoidCallWithGuard0 pins that a void go: call lowers through the
// statement form of the guard, so a call that returns nothing is protected the same
// way a value call is.
func TestGoImportGuardsVoidCallWithGuard0(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { GC } from "go:runtime";
GC();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "bridge.Guard0(func() {") {
		t.Errorf("a void go: call was not wrapped in the statement guard:\n%s", source)
	}
}

// TestGoImportPanicSurfacesAsCatchableError proves the boundary guard end to end: a
// go: call that panics inside the Go library (strings.Repeat with a negative count)
// converts to a thrown GoError that a bento try/catch recovers and reads as an Error
// whose message is the Go panic's string form, the section 12.3 guarantee that a Go
// panic becomes a catchable JavaScript exception.
func TestGoImportPanicSurfacesAsCatchableError(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: panic test builds and runs generated Go")
	}
	const src = `import { Repeat } from "go:strings";
try {
  Repeat("x", -1);
} catch (e) {
  if (e instanceof Error) {
    console.log("caught: " + e.message);
  }
}
`
	got := runProgramGo(t, src)
	if want := "caught: go: call panicked: strings: negative Repeat count\n"; got != want {
		t.Fatalf("go: panic program printed %q, want %q", got, want)
	}
}

// TestGoImportEmitsAliasedImport pins that the assembled Go imports the Go package
// under its alias and calls it qualified by that alias through the bridge, the
// shape section 9.1 fixes, without needing to run the program.
func TestGoImportEmitsAliasedImport(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { ToUpper } from "go:strings";
console.log(ToUpper("hi"));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, `strings "strings"`) {
		t.Errorf("assembled program does not import strings under its alias:\n%s", source)
	}
	if !strings.Contains(source, "strings.ToUpper(bridge.StringToGo(") {
		t.Errorf("assembled program does not call strings.ToUpper through the bridge:\n%s", source)
	}
	if !strings.Contains(source, "bridge.StringFromGo(") {
		t.Errorf("assembled program does not marshal the string result back through the bridge:\n%s", source)
	}
}
