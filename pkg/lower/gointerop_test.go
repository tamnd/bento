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

// TestGoImportMarshalsBytes proves the []byte crossing end to end in both
// directions (section 7.3). A Uint8Array built from a number list crosses into
// hex.EncodeToString as a Go []byte and the hex string crosses back, exercising
// the argument marshaling through bridge.BytesToGo. hex.DecodeString returns a
// ([]byte, error), so the error half unwraps into the throw machinery and the byte
// half crosses back through bridge.BytesFromGo into a Uint8Array whose length and
// bytes are then read. Running the binary is the oracle: the encode and the decode
// must round-trip through a real build the same way the standard library does.
func TestGoImportMarshalsBytes(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: bytes test builds and runs generated Go")
	}
	const src = `import { EncodeToString, DecodeString } from "go:encoding/hex";
const buf = new Uint8Array([65, 66, 67]);
console.log(EncodeToString(buf));
const back = DecodeString("48656c6c6f");
console.log(back.length);
console.log(back[0]);
console.log(back[4]);
`
	got := runProgramGo(t, src)
	if want := "414243\n5\n72\n111\n"; got != want {
		t.Fatalf("go: bytes program printed %q, want %q", got, want)
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

// TestGoImportMapResultEmitsEntryMarshaling pins that a go: call returning a map of
// basics lowers to bridge.MapFromGo over the Go call, with a per-entry key and value
// closure applying each element's own crossing into the empty bento Map its key kind
// fixes, and that a map argument lowers to bridge.MapToGo the same way (section 6.5).
func TestGoImportMapResultEmitsEntryMarshaling(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Counts, Total } from "go:github.com/tamnd/bento/pkg/goimport/mapfixture";
const counts = Counts("a b a");
console.log(Total(counts));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "bridge.MapFromGo(mapfixture.Counts(") {
		t.Errorf("a map result did not lower to entry marshaling:\n%s", source)
	}
	if !strings.Contains(source, "value.NewStringMap[float64]()") {
		t.Errorf("a map result did not build the string-keyed bento Map:\n%s", source)
	}
	if !strings.Contains(source, "bridge.MapToGo(") {
		t.Errorf("a map argument did not lower to entry marshaling:\n%s", source)
	}
	if !strings.Contains(source, "bridge.Guard(func() *value.Map[value.BStr, float64]") {
		t.Errorf("a map result was not guarded with the Map result type:\n%s", source)
	}
}

// TestGoImportMapRoundTripRuns proves the map crossing end to end: a program that
// counts words with mapfixture.Counts and sums the counts back with mapfixture.Total
// prints the total, so the map[string]int result, the bento Map consumption, and the
// map[string]int argument are all proven against a real build.
func TestGoImportMapRoundTripRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: map test builds and runs generated Go")
	}
	const src = `import { Counts, Total } from "go:github.com/tamnd/bento/pkg/goimport/mapfixture";
const counts = Counts("a b a c b a");
console.log(Total(counts));
`
	got := runProgramGo(t, src)
	if want := "6\n"; got != want {
		t.Fatalf("go: map program printed %q, want %q", got, want)
	}
}

// TestGoImportMapSizeRuns proves a map result is a real bento Map by reading its
// size: Counts over three distinct words builds a Map of three entries, and the
// program prints the size the Map model gives it.
func TestGoImportMapSizeRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: map test builds and runs generated Go")
	}
	const src = `import { Counts } from "go:github.com/tamnd/bento/pkg/goimport/mapfixture";
const counts = Counts("one two three two");
console.log(counts.size);
`
	got := runProgramGo(t, src)
	if want := "3\n"; got != want {
		t.Fatalf("go: map-size program printed %q, want %q", got, want)
	}
}

// TestGoImportStructResultEmitsInternedBox pins that a Go struct result lowers to a
// guarded closure that binds the Go value and returns a pointer to a freshly built
// interned struct, reading each exported Go field. The MakePoint result is a Point
// with X and Y, so the marshaling reads v.X and v.Y and boxes them into the same
// interned struct the interface shape interns to (sections 6.7 and 7.4).
func TestGoImportStructResultEmitsInternedBox(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { MakePoint } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
const p = MakePoint(3, 4);
console.log(p.X + p.Y);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "v := structfixture.MakePoint(") {
		t.Errorf("a struct result did not bind the Go call to v:\n%s", source)
	}
	if !strings.Contains(source, "v.X") || !strings.Contains(source, "v.Y") {
		t.Errorf("a struct result did not read each exported Go field:\n%s", source)
	}
	if !strings.Contains(source, "bridge.Guard(func() *") {
		t.Errorf("a struct result was not guarded with a pointer result type:\n%s", source)
	}
}

// TestGoImportStructResultRuns proves the struct crossing end to end: MakePoint
// returns a Go Point, the bento program reads p.X and p.Y off the boxed result, and
// their sum prints, so the Go struct value, the interned box, and the field access
// are all proven against a real build.
func TestGoImportStructResultRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: struct test builds and runs generated Go")
	}
	const src = `import { MakePoint } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
const p = MakePoint(3, 4);
console.log(p.X + p.Y);
`
	got := runProgramGo(t, src)
	if want := "7\n"; got != want {
		t.Fatalf("go: struct program printed %q, want %q", got, want)
	}
}

// TestGoImportStructMixedFieldsRun proves the crossing carries every basic field
// kind at once: MakeProfile returns a struct with a string name, a numeric age, and
// a boolean flag, and the bento program reads all three back, so each field marshals
// by its own keyword into the interned box.
func TestGoImportStructMixedFieldsRun(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: struct test builds and runs generated Go")
	}
	const src = `import { MakeProfile } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
const u = MakeProfile("ada", 36, true);
console.log(u.Name + " " + u.Age + " " + u.Active);
`
	got := runProgramGo(t, src)
	if want := "ada 36 true\n"; got != want {
		t.Fatalf("go: struct mixed-field program printed %q, want %q", got, want)
	}
}

// TestGoImportStructParamEmitsFieldMarshaling pins that a bento object passed to a Go
// struct parameter lowers to a closure that binds the box and returns a Go struct
// literal, reading each exported field off the box and marshaling it by keyword. Sum
// takes a Point, so the argument marshals into structfixture.Point{X: int(o.X), Y:
// int(o.Y)} at the call site (section 6.7).
func TestGoImportStructParamEmitsFieldMarshaling(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { MakePoint, Sum } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
const p = MakePoint(3, 4);
console.log(Sum(p));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func(o *ObjXY) structfixture.Point") {
		t.Errorf("a struct argument did not lower to a boxing closure:\n%s", source)
	}
	if !strings.Contains(source, "structfixture.Point{X: int(o.X), Y: int(o.Y)}") {
		t.Errorf("a struct argument did not build the Go struct literal from the box:\n%s", source)
	}
}

// TestGoImportStructParamRuns proves the struct-parameter crossing end to end: a Go
// Point built by MakePoint and a bento object literal both pass into Sum, which reads
// the fields on the Go side, so a struct crosses in from either a boxed result or a
// fresh object literal.
func TestGoImportStructParamRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: struct test builds and runs generated Go")
	}
	const src = `import { MakePoint, Sum, Describe } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
const p = MakePoint(3, 4);
console.log(Sum(p));
console.log(Sum({ X: 10, Y: 20 }));
console.log(Describe({ Name: "bob", Age: 5, Active: false }));
`
	got := runProgramGo(t, src)
	if want := "7\n30\nbob 5 inactive\n"; got != want {
		t.Fatalf("go: struct-parameter program printed %q, want %q", got, want)
	}
}

// TestGoImportStructPtrResultRuns proves a *Point result crosses back exactly like a
// value Point: MakePointPtr hands back a pointer, and the bento program reads p.X and
// p.Y off the same object box, because a field read auto-derefs on the Go side and a
// bento object is already a reference (section 6.6).
func TestGoImportStructPtrResultRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: struct test builds and runs generated Go")
	}
	const src = `import { MakePointPtr } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
const p = MakePointPtr(3, 4);
console.log(p.X + p.Y);
`
	got := runProgramGo(t, src)
	if want := "7\n"; got != want {
		t.Fatalf("go: *struct result program printed %q, want %q", got, want)
	}
}

// TestGoImportStructPtrParamEmitsAddressOf pins that a bento object passed to a Go
// *Point parameter lowers to a closure returning *structfixture.Point that takes the
// address of a fresh struct literal, so the pointer direction differs from the value
// direction only by the & and the *T result type.
func TestGoImportStructPtrParamEmitsAddressOf(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { SumPtr } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
console.log(SumPtr({ X: 10, Y: 20 }));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func(o *ObjXY) *structfixture.Point") {
		t.Errorf("a *struct argument did not lower to a pointer-returning closure:\n%s", source)
	}
	if !strings.Contains(source, "&structfixture.Point{X: int(o.X), Y: int(o.Y)}") {
		t.Errorf("a *struct argument did not take the address of the struct literal:\n%s", source)
	}
}

// TestGoImportStructPtrParamRuns proves the *Point-parameter crossing end to end: a
// fresh object literal passes into SumPtr, which reads the fields through the pointer
// on the Go side, so an object box crosses in as a &Point.
func TestGoImportStructPtrParamRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: struct test builds and runs generated Go")
	}
	const src = `import { MakePointPtr, SumPtr } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
const p = MakePointPtr(3, 4);
console.log(SumPtr(p));
console.log(SumPtr({ X: 10, Y: 20 }));
`
	got := runProgramGo(t, src)
	if want := "7\n30\n"; got != want {
		t.Fatalf("go: *struct-parameter program printed %q, want %q", got, want)
	}
}

// TestGoImportVariadicStructRuns proves the struct crossing composes with the
// variadic spread: SumAll takes any number of Points, and each object literal spread
// into it marshals through the struct crossing on its own, so the Go side reads every
// element's fields and the empty call reassembles an empty slice.
func TestGoImportVariadicStructRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: struct test builds and runs generated Go")
	}
	const src = `import { SumAll } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
console.log(SumAll({ X: 1, Y: 2 }, { X: 3, Y: 4 }));
console.log(SumAll());
`
	got := runProgramGo(t, src)
	if want := "10\n0\n"; got != want {
		t.Fatalf("go: variadic-struct program printed %q, want %q", got, want)
	}
}

// TestGoImportStructSliceResultEmitsArrayOfBoxes pins that a []struct result lowers
// to bridge.SliceFromGo over a per-element closure that boxes each Go struct into the
// interned pointer the array's element type interns to, wrapped in the boundary
// recover. Diagonal returns []Point, so each element crosses exactly as a lone Point
// result does (sections 6.4, 6.7).
func TestGoImportStructSliceResultEmitsArrayOfBoxes(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Diagonal } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
const pts = Diagonal(3);
console.log(pts.length);
console.log(pts[1].X + pts[1].Y);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "bridge.SliceFromGo(structfixture.Diagonal(") {
		t.Errorf("a []struct result did not marshal through SliceFromGo over the Go call:\n%s", source)
	}
	if !strings.Contains(source, "func(v structfixture.Point) *") {
		t.Errorf("a []struct result did not build a per-element boxing closure:\n%s", source)
	}
	if !strings.Contains(source, "bridge.Guard(func() *value.Array[*") {
		t.Errorf("a []struct result was not guarded with an array-of-boxes result type:\n%s", source)
	}
}

// TestGoImportStructSliceResultRuns proves the []struct crossing end to end: Diagonal
// returns three Points on y=x, the bento program reads the array's length and indexes
// an element to read its fields off the box, so the Go slice, the array of interned
// boxes, and the per-element field access are all proven against a real build.
func TestGoImportStructSliceResultRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: struct-slice test builds and runs generated Go")
	}
	const src = `import { Diagonal } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
const pts = Diagonal(3);
console.log(pts.length);
console.log(pts[1].X + pts[1].Y);
console.log(pts[2].X);
`
	got := runProgramGo(t, src)
	if want := "3\n2\n2\n"; got != want {
		t.Fatalf("go: struct-slice program printed %q, want %q", got, want)
	}
}

// TestGoImportStructSliceMixedFieldsRun proves the []struct crossing carries every
// basic field kind: Profiles returns a roster of Profiles with a string, a number,
// and a boolean field each, and the bento program reads all three off an indexed
// element, so each field marshals by its own keyword inside the array's boxes.
func TestGoImportStructSliceMixedFieldsRun(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: struct-slice test builds and runs generated Go")
	}
	const src = `import { Profiles } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
const rows = Profiles();
console.log(rows.length);
console.log(rows[0].Name + " " + rows[0].Age + " " + rows[0].Active);
console.log(rows[1].Name + " " + rows[1].Age + " " + rows[1].Active);
`
	got := runProgramGo(t, src)
	if want := "2\nada 36 true\nlinus 21 false\n"; got != want {
		t.Fatalf("go: struct-slice mixed-field program printed %q, want %q", got, want)
	}
}

// TestGoImportStructSliceParamEmitsSliceToGo pins that a bento array of objects
// passed to a Go []struct parameter lowers to bridge.SliceToGo over the same
// per-object boxing closure a lone struct argument builds, so each element marshals
// into structfixture.Point{X: int(o.X), Y: int(o.Y)} and SliceToGo collects the Go
// structs into the slice the call takes (sections 6.4, 6.7).
func TestGoImportStructSliceParamEmitsSliceToGo(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { SumSlice } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
console.log(SumSlice([{ X: 1, Y: 2 }, { X: 3, Y: 4 }]));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "bridge.SliceToGo(") {
		t.Errorf("a []struct argument did not marshal through SliceToGo:\n%s", source)
	}
	if !strings.Contains(source, "func(o *ObjXY) structfixture.Point") {
		t.Errorf("a []struct argument did not reuse the per-object boxing closure:\n%s", source)
	}
	if !strings.Contains(source, "structfixture.Point{X: int(o.X), Y: int(o.Y)}") {
		t.Errorf("a []struct argument did not build the Go struct literal from each box:\n%s", source)
	}
}

// TestGoImportStructSliceParamRuns proves the []struct argument crossing end to end:
// a bento array of point objects passes into SumSlice, which sums each element's
// fields on the Go side, and a single-element array crosses the same way, so the
// array of boxes, the per-element marshaling, and the collected slice are all proven
// against a real build.
func TestGoImportStructSliceParamRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: struct-slice test builds and runs generated Go")
	}
	const src = `import { SumSlice } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
console.log(SumSlice([{ X: 1, Y: 2 }, { X: 3, Y: 4 }]));
console.log(SumSlice([{ X: 5, Y: 6 }]));
`
	got := runProgramGo(t, src)
	if want := "10\n11\n"; got != want {
		t.Fatalf("go: struct-slice argument program printed %q, want %q", got, want)
	}
}

// TestGoImportStructSliceParamMixedFieldsRun proves the []struct argument crossing
// carries every basic field kind: DescribeAll takes a Go []Profile, and a bento
// array of objects with a string, a number, and a boolean field each crosses in so
// the Go side reads all three off every element and joins them.
func TestGoImportStructSliceParamMixedFieldsRun(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: struct-slice test builds and runs generated Go")
	}
	const src = `import { DescribeAll } from "go:github.com/tamnd/bento/pkg/goimport/structfixture";
const rows = [{ Name: "ada", Age: 36, Active: true }, { Name: "linus", Age: 21, Active: false }];
console.log(DescribeAll(rows));
`
	got := runProgramGo(t, src)
	if want := "ada 36 active, linus 21 inactive\n"; got != want {
		t.Fatalf("go: struct-slice mixed-field argument program printed %q, want %q", got, want)
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
	// The local folds to opt := ..., so its type is inferred from the OpaqueFromGo box
	// asserted just above rather than spelled out; seeing the short declaration for opt
	// confirms the boxed token is what the local binds.
	if !strings.Contains(source, "opt := bridge.") {
		t.Errorf("a local holding an opaque token was not bound to a bridge box:\n%s", source)
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
