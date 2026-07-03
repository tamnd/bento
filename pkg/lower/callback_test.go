package lower

import (
	"os/exec"
	"strings"
	"testing"
)

// TestGoImportCallbackWrapperShape pins the Go a callback argument lowers to: a
// wrapper func literal whose parameters are the Go func type's own types, each Go
// argument marshaled to the bento value the callback expects, the bento function
// called as a literal, and its result marshaled back to the Go return type (section
// 7.6). Reading the emitted code keeps the wrapper's shape visible in review without
// the toolchain.
func TestGoImportCallbackWrapperShape(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { Apply } from "go:github.com/tamnd/bento/pkg/goimport/funcfixture";
console.log(Apply(5, (n: number): number => n * 2));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"funcfixture.Apply(int(5), func(p0 int) int {",
		"return int(func(n float64) float64 {",
		"}(bridge.Int64ToNumber(int64(p0))))",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("callback wrapper missing %q:\n%s", want, source)
		}
	}
}

// TestGoImportCallbackApplyRuns proves the callback crossing end to end: Apply calls
// the bento function once with a Go int, and the doubled result crosses back to the
// Go int Apply returns, which the program prints.
func TestGoImportCallbackApplyRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: callback test builds and runs generated Go")
	}
	const src = `import { Apply } from "go:github.com/tamnd/bento/pkg/goimport/funcfixture";
console.log(Apply(5, (n: number): number => n * 2));
`
	got := runProgramGo(t, src)
	if want := "10\n"; got != want {
		t.Fatalf("go: callback program printed %q, want %q", got, want)
	}
}

// TestGoImportCallbackRepeatedRuns proves the wrapper survives repeated invocation:
// SumTo calls the bento function once per value in a Go loop and folds the results,
// so the printed total is the sum of what the callback returned for 0 through 3.
func TestGoImportCallbackRepeatedRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: callback test builds and runs generated Go")
	}
	const src = `import { SumTo } from "go:github.com/tamnd/bento/pkg/goimport/funcfixture";
console.log(SumTo(4, (i: number): number => i + 1));
`
	got := runProgramGo(t, src)
	if want := "10\n"; got != want {
		t.Fatalf("go: repeated-callback program printed %q, want %q", got, want)
	}
}

// TestGoImportCallbackMixedParamsRuns proves a callback with more than one parameter
// marshals each argument by its own type: Greet hands the callback a Go string and a
// Go int, and the built greeting crosses back as the Go string Greet returns.
func TestGoImportCallbackMixedParamsRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: callback test builds and runs generated Go")
	}
	const src = `import { Greet } from "go:github.com/tamnd/bento/pkg/goimport/funcfixture";
console.log(Greet("ada", 3, (name: string, times: number): string => name + "!" + times));
`
	got := runProgramGo(t, src)
	if want := "ada!3\n"; got != want {
		t.Fatalf("go: mixed-parameter callback program printed %q, want %q", got, want)
	}
}

// TestGoImportErrorCallbackWrapperShape pins the Go a throwing callback lowers to: a
// wrapper whose result is error and whose body runs the bento callback inside
// bridge.CallbackError, so a throw from the callback becomes the Go func's error
// return (section 7.6). The callback routes its conditional throw through a named
// function because a block-body arrow is a later slice, but the crossing is the same.
func TestGoImportErrorCallbackWrapperShape(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the checker needs it to generate go: declarations")
	}
	const src = `import { TryEach } from "go:github.com/tamnd/bento/pkg/goimport/funcfixture";
function boom(i: number): void { if (i === 2) { throw new Error("stop"); } }
try { TryEach(4, (i: number): void => boom(i)); } catch (e) { }
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"func(p0 int) error {",
		"return bridge.CallbackError(func() {",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("throwing-callback wrapper missing %q:\n%s", want, source)
		}
	}
}

// TestGoImportErrorCallbackRuns proves the throw-to-error crossing end to end: TryEach
// calls the callback until one returns an error and hands that error back. When the
// bento callback throws, the wrapper returns the thrown value's string form as a Go
// error, so TryEach stops and its (int, error) result hoists back to a bento throw the
// program catches. When the callback never throws, TryEach returns its count and nil,
// which the program prints.
func TestGoImportErrorCallbackRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: callback test builds and runs generated Go")
	}
	const src = `import { TryEach } from "go:github.com/tamnd/bento/pkg/goimport/funcfixture";
function boom(i: number): void { if (i === 2) { throw new Error("stop at " + i); } }
try {
  console.log(TryEach(4, (i: number): void => boom(i)));
} catch (e) {
  if (e instanceof Error) {
    console.log("caught: " + e.message);
  }
}
console.log(TryEach(2, (i: number): void => boom(i)));
`
	got := runProgramGo(t, src)
	if want := "caught: Error: stop at 2\n2\n"; got != want {
		t.Fatalf("go: throwing-callback program printed %q, want %q", got, want)
	}
}

// TestGoImportVoidCallbackRuns proves a void callback, a Go func(int) with no
// result, crosses and runs for its effect: Each calls the bento function once per
// value, and each call's console.log is observable as a printed line. The wrapper
// has no return, so the callback stands in the statement position.
func TestGoImportVoidCallbackRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the go: callback test builds and runs generated Go")
	}
	const src = `import { Each } from "go:github.com/tamnd/bento/pkg/goimport/funcfixture";
Each(3, (i: number): void => console.log(i));
`
	got := runProgramGo(t, src)
	if want := "0\n1\n2\n"; got != want {
		t.Fatalf("go: void-callback program printed %q, want %q", got, want)
	}
}
