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
