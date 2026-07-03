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

// testGoConstants is the constant resolver the program tests wire alongside
// testGoSignatures, loading each Go package's constants once and memoizing them, so
// a reference to a go: constant marshals by the real Go type the same way the build
// does. It is the test-side twin of build.goConstantResolver.
func testGoConstants() func(importPath, name string) (goimport.ConstInfo, bool) {
	memo := map[string]map[string]goimport.ConstInfo{}
	return func(importPath, name string) (goimport.ConstInfo, bool) {
		consts, loaded := memo[importPath]
		if !loaded {
			var err error
			consts, err = goimport.Constants(importPath)
			if err != nil {
				consts = map[string]goimport.ConstInfo{}
			}
			memo[importPath] = consts
		}
		info, ok := consts[name]
		return info, ok
	}
}

// testGoErrorVars is the sentinel-error resolver the program tests wire alongside
// the others, loading each Go package's error variables once and memoizing them, so
// a caught error's is() against a go: sentinel lowers the same way the build does.
// It is the test-side twin of build.goErrorVarResolver.
func testGoErrorVars() func(importPath, name string) bool {
	memo := map[string]map[string]bool{}
	return func(importPath, name string) bool {
		vars, loaded := memo[importPath]
		if !loaded {
			var err error
			vars, err = goimport.ErrorVars(importPath)
			if err != nil {
				vars = map[string]bool{}
			}
			memo[importPath] = vars
		}
		return vars[name]
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

// TestGoImportErrorIsEmitsIdentityCheck pins that a caught error's is() against a
// go: sentinel lowers to the value.Error identity methods of section 7.7: the
// instanceof GoError guard becomes IsGoError, and is(ErrSyntax) becomes Is against
// the qualified Go variable read, so the comparison is errors.Is against the real
// sentinel rather than a string match. The sentinel read pulls strconv.ErrSyntax
// straight from the Go package, bypassing the bento value model.
func TestGoImportErrorIsEmitsIdentityCheck(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Atoi, ErrSyntax } from "go:strconv";
import { GoError } from "bento:go";
try {
  Atoi("nope");
} catch (e) {
  if (e instanceof GoError) {
    if (e.is(ErrSyntax)) {
      console.log("syntax");
    }
  }
}
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".IsGoError()") {
		t.Errorf("instanceof GoError did not lower to the IsGoError narrowing:\n%s", source)
	}
	if !strings.Contains(source, ".Is(strconv.ErrSyntax)") {
		t.Errorf("err.is(ErrSyntax) did not lower to errors.Is against the sentinel:\n%s", source)
	}
}

// TestGoImportErrorIsRuns proves the error-identity path end to end: Atoi on a bad
// number returns a Go error wrapping strconv.ErrSyntax, and the caught error's
// is(ErrSyntax) reports true where is(ErrRange) reports false, so a bento author
// branches on Go error identity exactly as a Go author does with errors.Is
// (section 7.7).
func TestGoImportErrorIsRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: error identity test builds and runs generated Go")
	}
	const src = `import { Atoi, ErrSyntax, ErrRange } from "go:strconv";
import { GoError } from "bento:go";
try {
  Atoi("nope");
} catch (e) {
  if (e instanceof GoError) {
    console.log(e.is(ErrSyntax));
    console.log(e.is(ErrRange));
  }
}
`
	got := runProgramGo(t, src)
	if want := "true\nfalse\n"; got != want {
		t.Fatalf("go: error identity program printed %q, want %q", got, want)
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

// TestGoImportConstRefEmitsQualifiedRead pins that a reference to a go: constant
// lowers to the qualified Go constant read marshaled by its type: math.Pi is an
// untyped float that crosses as a plain float64 number, and math.MaxInt32 is an
// untyped int that crosses through the range check, the section 6.10 mapping. A
// constant read cannot panic, so it carries no boundary guard.
func TestGoImportConstRefEmitsQualifiedRead(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Pi, MaxInt32 } from "go:math";
console.log(Pi);
console.log(MaxInt32);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "math.Pi") {
		t.Errorf("a float constant reference did not lower to the qualified read:\n%s", source)
	}
	if !strings.Contains(source, "bridge.Int64ToNumber(int64(math.MaxInt32))") {
		t.Errorf("an int constant reference did not cross through the range check:\n%s", source)
	}
	if strings.Contains(source, "bridge.Guard(func() float64 { return math.Pi") {
		t.Errorf("a constant read was wrapped in the boundary guard, which it does not need:\n%s", source)
	}
}

// TestGoImportConstRefRuns proves the constant crossing end to end: a program that
// reads math.Pi and math.MaxInt32 prints the values the TypeScript would, so the
// qualified read and the number marshaling are proven against a real build.
func TestGoImportConstRefRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: constant test builds and runs generated Go")
	}
	const src = `import { Pi, MaxInt32 } from "go:math";
console.log(Pi);
console.log(MaxInt32);
`
	got := runProgramGo(t, src)
	if want := "3.141592653589793\n2147483647\n"; got != want {
		t.Fatalf("go: constant program printed %q, want %q", got, want)
	}
}

// TestGoImportDefinedConstStripsBrand pins that a reference to a go: constant of a
// defined type over a basic (time.Second is a time.Duration over int64) lowers to
// the qualified read converted back to the underlying number before it crosses, so
// the branded value passes through as the plain number the projection promises, the
// section 6.11 mapping for a defined-type constant.
func TestGoImportDefinedConstStripsBrand(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Second } from "go:time";
console.log(Second);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "bridge.Int64ToNumber(int64(time.Second))") {
		t.Errorf("a defined-type constant did not strip its brand into the underlying number:\n%s", source)
	}
}

// TestGoImportDefinedConstRuns proves the defined-type constant crossing end to end:
// a program that reads time.Second and time.Millisecond prints their underlying
// nanosecond counts, so the qualified read, the brand strip, and the number
// marshaling are proven against a real build.
func TestGoImportDefinedConstRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: defined-const test builds and runs generated Go")
	}
	const src = `import { Second, Millisecond } from "go:time";
console.log(Second);
console.log(Millisecond);
`
	got := runProgramGo(t, src)
	if want := "1000000000\n1000000\n"; got != want {
		t.Fatalf("go: defined-const program printed %q, want %q", got, want)
	}
}

// TestGoImportDefinedParamConvertsToNamedType pins that a go: call whose parameter is
// a defined type over a basic converts the crossed number to the named Go type on the
// way in: time.Sleep takes a time.Duration, so the marshaled int64 is wrapped in
// time.Duration(...) before the call, the section 6.11 mapping for a defined-type
// parameter. The argument is the Duration-branded Millisecond constant, which the
// projection admits where a plain number would not type-check.
func TestGoImportDefinedParamConvertsToNamedType(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Sleep, Millisecond } from "go:time";
Sleep(Millisecond);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "time.Sleep(time.Duration(") {
		t.Errorf("a defined-type parameter did not convert the crossed number to the named type:\n%s", source)
	}
}

// TestGoImportSliceResultEmitsElementMarshaling pins that a go: call returning a
// slice of a basic lowers to bridge.SliceFromGo over the Go call, with a per-element
// closure applying the element's own crossing, and that a slice argument lowers to
// bridge.SliceToGo the same way (section 6.4).
func TestGoImportSliceResultEmitsElementMarshaling(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Split, Join } from "go:strings";
const parts = Split("a,b,c", ",");
console.log(Join(parts, "-"));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "bridge.SliceFromGo(strings.Split(") {
		t.Errorf("a slice result did not lower to element marshaling:\n%s", source)
	}
	if !strings.Contains(source, "bridge.StringFromGo(x)") {
		t.Errorf("the slice element closure did not reuse the string crossing:\n%s", source)
	}
	if !strings.Contains(source, "bridge.SliceToGo(") {
		t.Errorf("a slice argument did not lower to element marshaling:\n%s", source)
	}
	if !strings.Contains(source, "bridge.Guard(func() *value.Array[value.BStr]") {
		t.Errorf("a slice result was not guarded with the array result type:\n%s", source)
	}
}

// TestGoImportSliceRoundTripRuns proves the slice crossing end to end: a program that
// splits a string into an array with strings.Split and joins it back with
// strings.Join prints the joined string, so the []string result, the array
// consumption, and the []string argument are all proven against a real build.
func TestGoImportSliceRoundTripRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: slice test builds and runs generated Go")
	}
	const src = `import { Split, Join } from "go:strings";
const parts = Split("a,b,c", ",");
console.log(Join(parts, "-"));
`
	got := runProgramGo(t, src)
	if want := "a-b-c\n"; got != want {
		t.Fatalf("go: slice program printed %q, want %q", got, want)
	}
}

// TestGoImportSliceLengthRuns proves a slice result is a real bento array by reading
// its length: Fields splits on whitespace and the program prints the count, so the
// array the crossing produces carries its own length the array model gives it.
func TestGoImportSliceLengthRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: slice test builds and runs generated Go")
	}
	const src = `import { Fields } from "go:strings";
const words = Fields("one two three");
console.log(words.length);
`
	got := runProgramGo(t, src)
	if want := "3\n"; got != want {
		t.Fatalf("go: slice-length program printed %q, want %q", got, want)
	}
}

// TestGoImportVariadicEmitsSpreadCall pins that a call to a variadic Go function
// spreads its arguments positionally into the Go call: path.Join is func(...string),
// so each argument marshals through StringToGo on its own and Go reassembles the
// slice from the positional arguments, with no bento-side slice built (section 6.9).
func TestGoImportVariadicEmitsSpreadCall(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Join } from "go:path";
console.log(Join("usr", "local", "bin"));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "path.Join(bridge.StringToGo(") {
		t.Errorf("a variadic call did not spread its first argument through the bridge into the Go call:\n%s", source)
	}
	if n := strings.Count(source, "bridge.StringToGo("); n != 3 {
		t.Errorf("variadic call marshaled %d string arguments, want one per spread argument (3):\n%s", n, source)
	}
}

// TestGoImportVariadicRuns proves the variadic crossing end to end: path.Join with
// three arguments joins them with a slash, and path.Join with no arguments returns
// the empty string, so a variadic call marshals each argument as one element and the
// zero-argument case emits a bare call the Go side reassembles into an empty slice.
func TestGoImportVariadicRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: variadic test builds and runs generated Go")
	}
	const src = `import { Join } from "go:path";
console.log(Join("usr", "local", "bin"));
console.log(Join());
`
	got := runProgramGo(t, src)
	if want := "usr/local/bin\n\n"; got != want {
		t.Fatalf("go: variadic program printed %q, want %q", got, want)
	}
}

// TestGoImportVariadicMixedFixedRuns proves a fixed parameter ahead of a variadic
// tail marshals by its own type while the tail marshals by the element type:
// fmt.Sprintf takes a fixed format string and a variadic ...any, so the format
// crosses through the string bridge and each trailing argument boxes through the any
// crossing, and the formatted result crosses back (sections 6.9, 6.12).
func TestGoImportVariadicMixedFixedRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: variadic test builds and runs generated Go")
	}
	const src = `import { Sprintf } from "go:fmt";
console.log(Sprintf("%s and %s", "cats", "dogs"));
`
	got := runProgramGo(t, src)
	if want := "cats and dogs\n"; got != want {
		t.Fatalf("go: mixed fixed-and-variadic program printed %q, want %q", got, want)
	}
}

// TestGoImportOpaqueHandleEmitsTokenCrossing pins that a go: call returning a type
// the bridge does not project boxes it into the uniform bridge.Opaque token: the
// result is wrapped by OpaqueFromGo, a local that holds it is typed bridge.Opaque
// whatever the foreign type is, and passing the token back recovers the concrete
// type through OpaqueToGo named by its package alias, the section 6.13 crossing.
func TestGoImportOpaqueHandleEmitsTokenCrossing(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { WithLevel, Describe } from "go:github.com/tamnd/bento/pkg/goimport/optfixture";
const opt = WithLevel(7);
console.log(Describe(opt));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "bridge.OpaqueFromGo(optfixture.WithLevel(") {
		t.Errorf("an opaque result was not boxed into a token:\n%s", source)
	}
	if !strings.Contains(source, "var opt bridge.Opaque = ") {
		t.Errorf("a local holding an opaque token was not typed bridge.Opaque:\n%s", source)
	}
	if !strings.Contains(source, "bridge.OpaqueToGo[optfixture.Level](opt)") {
		t.Errorf("an opaque argument did not recover its concrete Go type on the way back:\n%s", source)
	}
}

// TestGoImportOpaqueHandleRoundTripRuns proves the opaque crossing end to end: the
// program receives an option token from one go: call and hands it to another, which
// reads the level it carries, so the token round-trips through bento untouched and
// the second call prints the number the first put in.
func TestGoImportOpaqueHandleRoundTripRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: opaque test builds and runs generated Go")
	}
	const src = `import { WithLevel, Describe } from "go:github.com/tamnd/bento/pkg/goimport/optfixture";
const opt = WithLevel(7);
console.log(Describe(opt));
`
	got := runProgramGo(t, src)
	if want := "7\n"; got != want {
		t.Fatalf("go: opaque round-trip printed %q, want %q", got, want)
	}
}

// TestGoImportAnyEmitsBoxCrossing pins that a go: call with an any parameter and an
// any result boxes and unboxes through the value model: a statically typed argument
// is lifted to a value.Value and marshaled through bridge.AnyToGo, and the any result
// crosses back through bridge.AnyFromGo, the section 6.12 crossing.
func TestGoImportAnyEmitsBoxCrossing(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Echo, Name } from "go:github.com/tamnd/bento/pkg/goimport/anyfixture";
console.log(Name(Echo(42)));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "bridge.AnyToGo(value.Number(") {
		t.Errorf("a statically typed any argument was not boxed and marshaled:\n%s", source)
	}
	if !strings.Contains(source, "bridge.AnyFromGo(anyfixture.Echo(") {
		t.Errorf("an any result was not unboxed through the bridge:\n%s", source)
	}
}

// TestGoImportAnyRoundTripRuns proves the any crossing end to end: Echo hands a value
// through a Go any and back, and Name reports the Go kind the crossing unwrapped it
// to, so a number stays a number across the round trip and a string reports as a
// string, both observed against a real build.
func TestGoImportAnyRoundTripRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: any test builds and runs generated Go")
	}
	const src = `import { Echo, Name } from "go:github.com/tamnd/bento/pkg/goimport/anyfixture";
console.log(Name(Echo(3)));
console.log(Name("hi"));
console.log(Name(true));
`
	got := runProgramGo(t, src)
	if want := "number\nstring\nbool\n"; got != want {
		t.Fatalf("go: any round-trip printed %q, want %q", got, want)
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

// TestGoImportPathsReportsReachedPackages proves GoImportPaths lists the Go import
// path a go: call reached, the seam the build reads to detect cgo before it runs
// the toolchain (section 9.5). A program that calls strings.ToUpper reaches exactly
// the strings package, so the accessor returns it.
func TestGoImportPathsReportsReachedPackages(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { ToUpper } from "go:strings";
console.log(ToUpper("hi"));
`
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	if _, err := r.RenderProgram(entryFile(t, prog)); err != nil {
		t.Fatalf("RenderProgram: %v", err)
	}
	if paths := r.GoImportPaths(); len(paths) != 1 || paths[0] != "strings" {
		t.Errorf("GoImportPaths() = %v, want [strings]", paths)
	}
}

// TestGoImportPathsEmptyWithoutGoImport proves a program that reaches no go: import
// reports no interop paths, so the cgo gate sees nothing to check and a plain
// program never pays a detection load. It needs no go toolchain since it lowers no
// go: call.
func TestGoImportPathsEmptyWithoutGoImport(t *testing.T) {
	prog := compile(t, "console.log(1 + 2);\n")
	r := NewRenderer(prog)
	if _, err := r.RenderProgram(entryFile(t, prog)); err != nil {
		t.Fatalf("RenderProgram: %v", err)
	}
	if paths := r.GoImportPaths(); len(paths) != 0 {
		t.Errorf("GoImportPaths() = %v, want empty for a program with no go: import", paths)
	}
}
