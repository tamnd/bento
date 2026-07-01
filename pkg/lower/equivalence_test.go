package lower

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/engine"
	_ "github.com/tamnd/bento/pkg/engine/quickjs" // registers the default backend
	"github.com/tamnd/bento/pkg/frontend"
)

// This file proves the compiler is sound the only way that counts: it runs the
// same function two ways and checks the answers match. The reference is the
// TypeScript itself, transpiled to JavaScript and executed by bento's engine;
// the subject is the Go the lowerer emits, compiled and run by the Go toolchain.
// A case passes only when both produce the same number for the same arguments,
// so a mistranslation in the mapping table shows up as a diverging result rather
// than passing silently. This is the "TypeScript and generated Go are identical"
// guarantee the directive asks for, mechanized.

// equivCase is one function exercised with several argument tuples. The source
// defines one exported function; the calls drive it with concrete numbers.
type equivCase struct {
	name string
	src  string
	fn   string
	args [][]float64
}

// TestTSAndGeneratedGoAgree runs each case through both execution paths and
// fails on any divergence. It is skipped when the Go toolchain is not on PATH,
// because the subject side compiles and runs real Go.
func TestTSAndGeneratedGoAgree(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; equivalence test needs it to run generated Go")
	}

	cases := []equivCase{
		{
			name: "identity",
			src:  "export function identity(x: number): number { return x; }",
			fn:   "identity",
			args: [][]float64{{0}, {42}, {-7}, {3.5}},
		},
		{
			name: "add",
			src:  "export function add(a: number, b: number): number { return a + b; }",
			fn:   "add",
			args: [][]float64{{1, 2}, {-3, 3}, {0.1, 0.2}, {1e6, 1}},
		},
		{
			name: "arithmetic",
			src:  "export function mix(a: number, b: number): number { return (a + b) * a - b / 2; }",
			fn:   "mix",
			args: [][]float64{{2, 4}, {5, 10}, {-1, -2}, {1.5, 0.5}},
		},
		{
			name: "keywordParam",
			src:  "export function pick(type: number): number { return type; }",
			fn:   "pick",
			args: [][]float64{{9}, {-4}},
		},
		{
			name: "loop",
			src: `export function score(n: number): number {
  let total = 0;
  let i = 1;
  while (i <= n) {
    if (i === 3) {
      total = total + 10;
    } else {
      total = total + i;
    }
    i = i + 1;
  }
  return total;
}`,
			fn:   "score",
			args: [][]float64{{0}, {1}, {3}, {5}, {10}},
		},
		{
			name: "factorial",
			src: `export function fact(n: number): number {
  let acc = 1;
  let i = 2;
  while (i <= n) {
    acc = acc * i;
    i = i + 1;
  }
  return acc;
}`,
			fn:   "fact",
			args: [][]float64{{0}, {1}, {5}, {10}},
		},
		{
			name: "branch",
			src: `export function clamp(x: number): number {
  if (x < 0) {
    return 0;
  } else if (x > 100) {
    return 100;
  }
  return x;
}`,
			fn:   "clamp",
			args: [][]float64{{-5}, {0}, {42}, {100}, {250}},
		},
		{
			name: "recursion",
			src: `export function fib(n: number): number {
  if (n < 2) {
    return n;
  }
  return fib(n - 1) + fib(n - 2);
}`,
			fn:   "fib",
			args: [][]float64{{0}, {1}, {7}, {12}},
		},
		{
			name: "forLoopNegate",
			src: `export function negsum(n: number): number {
  let t = 0;
  for (let i = 1; i <= n; i = i + 1) {
    t = t + i;
  }
  return -t;
}`,
			fn:   "negsum",
			args: [][]float64{{0}, {4}, {10}},
		},
		{
			name: "stringLength",
			// The literal holds an astral character (one rune, two UTF-16 code
			// units). .length must report code units, so the whole string is
			// "a" (1) + emoji (2) + "b" (1) = 4. A Go len() over UTF-8 bytes would
			// say 6, so this proves the emitted Go runs the real value.BStr and
			// gets the JavaScript answer, matched against quickjs. No arguments,
			// because the harness passes numbers; string arguments are a later
			// slice once the harness carries typed argument tuples.
			src: `export function width(): number {
  let s = "a" + "😀" + "b";
  return s.length;
}`,
			fn:   "width",
			args: [][]float64{{}},
		},
		{
			name: "modulo",
			src:  "export function rem(a: number, b: number): number { return a % b; }",
			// fmod keeps the sign of the dividend and works on fractions, so the
			// cases cover negative dividends and a non-integer operand, where a
			// naive integer remainder would diverge from JavaScript.
			fn:   "rem",
			args: [][]float64{{7, 3}, {-7, 3}, {7, -3}, {5.5, 2}, {10, 10}},
		},
		{
			name: "logical",
			src: `export function between(x: number, lo: number, hi: number): number {
  if (x >= lo && x <= hi) {
    return 1;
  }
  if (x < lo || x > hi) {
    return -1;
  }
  return 0;
}`,
			fn:   "between",
			args: [][]float64{{5, 0, 10}, {-1, 0, 10}, {11, 0, 10}, {0, 0, 10}, {10, 0, 10}},
		},
		{
			name: "crossCall",
			src: `export function square(x: number): number {
  return x * x;
}
export function hypotSq(a: number, b: number): number {
  return square(a) + square(b);
}`,
			fn:   "hypotSq",
			args: [][]float64{{3, 4}, {0, 0}, {-2, 5}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goSrc, imports := lowerToGo(t, tc.src)
			goName, ok := exportedField(tc.fn)
			if !ok {
				t.Fatalf("entry %q is not a Go identifier", tc.fn)
			}
			for _, args := range tc.args {
				want := evalTS(t, tc.src, tc.fn, args)
				got := runGo(t, goSrc, imports, goName, args)
				if got != want {
					t.Errorf("%s(%v): generated Go = %v, TypeScript = %v", tc.fn, args, got, want)
				}
			}
		})
	}
}

// lowerToGo compiles the snippet and lowers every top-level function plus any
// generated type declarations, returning the combined Go source. Lowering all
// functions (not just the entry) lets a case call a helper or recurse. A
// hand-back here is a test failure: every equivalence case is inside the
// lowerable subset by construction.
func lowerToGo(t *testing.T, src string) (string, []string) {
	t.Helper()
	prog := compile(t, src)
	var fns []frontend.Node
	collectKind(prog, prog.SourceFiles(), frontend.NodeFunctionDeclaration, &fns)
	if len(fns) == 0 {
		t.Fatal("no function declaration in snippet")
	}
	r := NewRenderer(prog)
	funcs := make([]string, 0, len(fns))
	for _, fn := range fns {
		decl, err := r.RenderFunc(fn)
		if err != nil {
			t.Fatalf("RenderFunc: %v", err)
		}
		funcs = append(funcs, decl.Source)
	}
	var b strings.Builder
	for _, d := range r.Decls() { // struct and enum decls the functions referenced
		b.WriteString(d.Source)
		b.WriteByte('\n')
	}
	for _, f := range funcs {
		b.WriteString(f)
		b.WriteByte('\n')
	}
	return b.String(), r.Imports()
}

// evalTS transpiles the TypeScript to JavaScript, evaluates it in the engine,
// and calls the function with the given arguments, returning the number the
// engine produced. This is the reference: the source's own runtime meaning.
func evalTS(t *testing.T, src, fn string, args []float64) float64 {
	t.Helper()
	// The engine evaluates the transpiled source as a global script, so the
	// function must stay a top-level declaration rather than a module export;
	// dropping the export keyword keeps identical runtime behavior without
	// pulling in a CommonJS module scope the bare Eval does not provide.
	global := strings.ReplaceAll(src, "export ", "")
	js, err := frontend.Transpile(global, frontend.Options{Filename: "m.ts"})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	eng, err := engine.New("")
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer func() { _ = eng.Close() }()
	if _, err := eng.Eval("m.js", js.Code); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	callArgs := make([]any, len(args))
	for i, a := range args {
		callArgs[i] = a
	}
	res, err := eng.Call(fn, callArgs...)
	if err != nil {
		t.Fatalf("Call %s: %v", fn, err)
	}
	f, ok := toFloat(res)
	if !ok {
		t.Fatalf("engine returned %T (%v), want a number", res, res)
	}
	return f
}

// runGo wraps the generated function in a tiny main, compiles and runs it with
// the Go toolchain, and parses the number it prints. This is the subject: what
// the emitted Go actually computes, not what we hope it computes.
func runGo(t *testing.T, goSrc string, imports []string, name string, args []float64) float64 {
	t.Helper()
	dir := t.TempDir()

	callArgs := make([]string, len(args))
	for i, a := range args {
		callArgs[i] = strconv.FormatFloat(a, 'g', -1, 64)
	}
	// The wrapper always prints with fmt; the lowered code adds whatever else it
	// referenced (math for a % that became math.Mod, and later the value model).
	paths := append([]string{"fmt"}, imports...)
	var imp strings.Builder
	imp.WriteString("import (\n")
	for _, p := range paths {
		imp.WriteString("\t\"" + p + "\"\n")
	}
	imp.WriteString(")")
	main := fmt.Sprintf(
		"package main\n\n%s\n\n%s\nfunc main() {\n\tfmt.Printf(\"%%g\\n\", %s(%s))\n}\n",
		imp.String(), goSrc, name, strings.Join(callArgs, ", "),
	)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod(t, imports)), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run failed: %v\n--- program ---\n%s\n--- output ---\n%s", err, main, out)
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		t.Fatalf("parse go output %q: %v", out, err)
	}
	return f
}

// goMod builds the throwaway module's go.mod. When the lowered code imports a
// bento runtime package (the value model, for a string program), the module
// requires bento and replaces it with the working copy on disk, so the generated
// Go links against the very value.BStr this repo defines rather than a published
// version. A program that touches no runtime package needs neither line and gets
// a bare module, so the common numeric case does not pay for a module download.
func goMod(t *testing.T, imports []string) string {
	t.Helper()
	needsBento := false
	for _, p := range imports {
		if strings.HasPrefix(p, "github.com/tamnd/bento") {
			needsBento = true
		}
	}
	var b strings.Builder
	b.WriteString("module eqtest\n\ngo 1.26\n")
	if needsBento {
		// The test runs with its working directory at the package (pkg/lower), so
		// the repo root is two levels up; the replace needs an absolute path.
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		root, err := filepath.Abs(filepath.Join(wd, "..", ".."))
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString("\nrequire github.com/tamnd/bento v0.0.0\n")
		b.WriteString("\nreplace github.com/tamnd/bento => " + root + "\n")
	}
	return b.String()
}

// toFloat coerces the engine's native return value to a float64. The quickjs
// backend hands numbers back as float64, but an integer-valued result can arrive
// as an int, so both are accepted.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
