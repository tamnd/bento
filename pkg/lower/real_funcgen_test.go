package lower

import (
	"errors"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// firstFunc compiles src and returns the program plus its first function
// declaration node, the unit RenderFunc lowers.
func firstFunc(t *testing.T, src string) (*frontend.Program, frontend.Node) {
	t.Helper()
	prog := compile(t, src)
	var fns []frontend.Node
	collectKind(prog, prog.SourceFiles(), frontend.NodeFunctionDeclaration, &fns)
	if len(fns) == 0 {
		t.Fatal("no function declaration in snippet")
	}
	return prog, fns[0]
}

// renderFunc compiles src, lowers its first function, and returns the generated
// Go declaration source (or the hand-back error).
func renderFunc(t *testing.T, src string) (*Renderer, string, error) {
	t.Helper()
	prog, fn := firstFunc(t, src)
	r := NewRenderer(prog)
	decl, err := r.RenderFunc(fn)
	return r, decl.Source, err
}

// TestRenderFuncGoldens drives the statement-and-expression slice end to end: a
// real TypeScript function is checked, lowered, and printed to a Go declaration,
// pinned by a checked-in golden so the exact generated code is visible in review.
// Each case pairs a .ts input (the src) with a .go golden, the "original
// TypeScript to generated Go" proof the directive asks for.
func TestRenderFuncGoldens(t *testing.T) {
	cases := []struct {
		name, golden, src string
	}{
		{
			name:   "identity",
			golden: "func_identity.golden",
			src:    "export function identity(x: number): number { return x; }",
		},
		{
			name:   "add",
			golden: "func_add.golden",
			src:    "export function add(a: number, b: number): number { return a + b; }",
		},
		{
			name:   "arithmetic",
			golden: "func_arithmetic.golden",
			// exercises every mapped operator plus parentheses and a numeric
			// literal, all on float64.
			src: "export function mix(a: number, b: number): number { return (a + b) * a - b / 2; }",
		},
		{
			name:   "keyword_param",
			golden: "func_keyword_param.golden",
			// a parameter named for a Go keyword must mangle to type_ on both
			// its declaration and its use, and stay consistent between them.
			src: "export function pick(type: number): number { return type; }",
		},
		{
			name:   "loop",
			golden: "func_loop.golden",
			// exercises local var declarations, a while loop, an if/else, a
			// relational and a strict-equality condition, and assignment, so a
			// whole control-flow body is pinned as one golden.
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
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got, err := renderFunc(t, tc.src)
			if err != nil {
				t.Fatalf("RenderFunc(%s): %v", tc.name, err)
			}
			checkGolden(t, tc.golden, got)
		})
	}
}

// TestRenderFuncVoidReturn pins that a function with no return value lowers to a
// Go function with no result list.
func TestRenderFuncVoidReturn(t *testing.T) {
	_, got, err := renderFunc(t, "export function noop(x: number): void { return; }")
	if err != nil {
		t.Fatalf("RenderFunc(void): %v", err)
	}
	checkGolden(t, "func_void.golden", got)
}

// TestRenderFuncHandsBack pins the section 30 boundary for function bodies: any
// construct outside the covered statement and expression subset returns a
// NotYetLowerable rather than wrong or incomplete Go.
func TestRenderFuncHandsBack(t *testing.T) {
	cases := []struct{ name, src string }{
		// % is fmod on JS numbers, not Go's integer remainder, so it waits for
		// its own slice.
		{"modulo", "export function m(a: number, b: number): number { return a % b; }"},
		// + on strings is concatenation of a different type.
		{"stringConcat", "export function c(a: string, b: string): string { return a + b; }"},
		// a truthy number condition needs JavaScript coercion, not a Go bool.
		{"truthyCond", "export function t(a: number): number { if (a) { return 1; } return 0; }"},
		// a string-keyed for-of and other loops beyond while are later slices.
		{"forLoop", "export function s(n: number): number { let t = 0; for (let i = 0; i < n; i = i + 1) { t = t + i; } return t; }"},
		// a generic function needs monomorphization first.
		{"generic", "export function id<T>(x: T): T { return x; }"},
		// an optional parameter needs the optional tagged type.
		{"optionalParam", "export function o(a: number, b?: number): number { return a; }"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := renderFunc(t, tc.src)
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderFunc(%s) err = %v, want a *NotYetLowerable", tc.name, err)
			}
		})
	}
}
