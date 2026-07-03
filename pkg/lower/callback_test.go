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
