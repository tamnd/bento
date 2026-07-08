package lower

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestNullishCoalesceEmits pins the shape of the lowering: an optional left with
// a pure fallback becomes an Or call on the Opt, a fallback that is itself
// optional becomes OrOpt so the result stays optional, and a dynamic left becomes
// a value.Coalesce over both boxed operands, whose nullish test is the runtime
// presence check rather than an Opt flag.
func TestNullishCoalesceEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"valueFallback",
			"function f(x: number | undefined): number { return x ?? 0; }\nconsole.log(f(undefined));\n",
			"x.Or(0)",
		},
		{
			"stringFallback",
			"function f(x: string | undefined): string { return x ?? \"none\"; }\nconsole.log(f(undefined));\n",
			"x.Or(value.FromGoString(\"none\"))",
		},
		{
			"optionalFallback",
			"function f(a: number | undefined, b: number | undefined): number | undefined { return a ?? b; }\nconsole.log(f(undefined, undefined) ?? -1);\n",
			"a.OrOpt(b)",
		},
		{
			"dynamicLeft",
			"function f(x: any): any { return x ?? 0; }\nconsole.log(f(undefined));\n",
			"value.Coalesce(x, value.Number(0))",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("nullish coalescing did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestNullishCoalesceLazyEmits pins the lazy-closure shape a side-effecting
// fallback lowers to: the optional left binds to a temp and its presence is the
// IsUndefined test, the dynamic left binds and tests IsNullish, and in both cases
// the fallback call sits inside the closure body so it runs only on the nullish
// branch. This is what keeps ??'s short-circuit when the fallback is not pure.
func TestNullishCoalesceLazyEmits(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		wants []string
	}{
		{
			"optionalLeft",
			"function side(): number { return 1; }\nfunction f(x: number | undefined): number { return x ?? side(); }\nconsole.log(f(undefined));\n",
			[]string{".IsUndefined()", "Side()", ".Get()"},
		},
		{
			"dynamicLeft",
			"function side(): any { return 1; }\nfunction f(x: any): any { return x ?? side(); }\nconsole.log(f(undefined));\n",
			[]string{".IsNullish()", "Side()"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			for _, want := range tc.wants {
				if !strings.Contains(source, want) {
					t.Errorf("lazy nullish coalescing did not print %q:\n%s", want, source)
				}
			}
		})
	}
}

// TestNullishCoalesceHandsBack pins the one ?? boundary that still hands back: a
// dynamic fallback into an optional left mixes the two nullish representations and
// has no bridge yet, so it names its own later slice.
func TestNullishCoalesceHandsBack(t *testing.T) {
	const src = "function side(): any { return 1; }\nfunction f(x: number | undefined): number { return x ?? side(); }\nconsole.log(f(undefined));\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "dynamic fallback") {
		t.Errorf("hand-back reason = %q, want it to contain %q", nyl.Reason, "dynamic fallback")
	}
}

// TestNullishCoalesceLazyRuns builds and runs a side-effecting fallback and
// checks both the value and the short-circuit: a present left keeps its value and
// the fallback's console.log never fires, an undefined left falls to the fallback
// and does fire, and the dynamic left keeps the same contract over boxed values.
func TestNullishCoalesceLazyRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the test builds and runs generated Go")
	}
	const src = `function loud(tag: string, v: number): number {
  console.log("fb:" + tag);
  return v;
}
function opt(x: number | undefined, tag: string): number {
  return x ?? loud(tag, -1);
}
function dyn(x: any, tag: string): any {
  return x ?? loud(tag, -2);
}
console.log(opt(5, "a"));
console.log(opt(undefined, "b"));
console.log(dyn(0, "c"));
console.log(dyn(undefined, "d"));
`
	got := runProgramGo(t, src)
	want := "5\n" +
		"fb:b\n" +
		"-1\n" +
		"0\n" +
		"fb:d\n" +
		"-2\n"
	if got != want {
		t.Fatalf("lazy nullish program printed %q, want %q", got, want)
	}
}

// TestNullishCoalesceRuns builds and runs nullish coalescing end to end: a
// present optional keeps its value, an undefined one falls to the fallback, a
// falsy-but-present value (zero, empty string) is kept rather than replaced (the
// difference between ?? and ||), and an optional fallback chains.
func TestNullishCoalesceRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the test builds and runs generated Go")
	}
	const src = `function num(x: number | undefined): number {
  return x ?? -1;
}
function str(s: string | undefined): string {
  return s ?? "default";
}
function chain(a: number | undefined, b: number | undefined): number {
  return (a ?? b) ?? -2;
}
console.log(num(5));
console.log(num(0));
console.log(num(undefined));
console.log(str("hi"));
console.log(str(""));
console.log(str(undefined));
console.log(chain(undefined, 7));
console.log(chain(undefined, undefined));
`
	got := runProgramGo(t, src)
	want := "5\n" +
		"0\n" +
		"-1\n" +
		"hi\n" +
		"\n" +
		"default\n" +
		"7\n" +
		"-2\n"
	if got != want {
		t.Fatalf("nullish program printed %q, want %q", got, want)
	}
}

// TestDynamicNullishCoalesceRuns builds and runs ?? on a dynamic left, the shape
// the value.Coalesce path lowers. The runtime tests presence, not truthiness, so
// a present zero or empty string is kept while null and undefined fall to the
// fallback, the same ?? contract the optional path keeps but over boxed values.
func TestDynamicNullishCoalesceRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the test builds and runs generated Go")
	}
	const src = `function pick(x: any, fb: any): any {
  return x ?? fb;
}
console.log(pick(0, 99));
console.log(pick(null, 99));
console.log(pick(undefined, 7));
console.log(pick("", "z"));
console.log(pick("kept", "z"));
`
	got := runProgramGo(t, src)
	want := "0\n" +
		"99\n" +
		"7\n" +
		"\n" +
		"kept\n"
	if got != want {
		t.Fatalf("dynamic nullish program printed %q, want %q", got, want)
	}
}
