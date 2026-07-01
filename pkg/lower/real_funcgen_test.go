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
		{
			name:   "recursion",
			golden: "func_fib.golden",
			// a self-recursive call resolves to the same exported Go name the
			// declaration gets, so the call and its target agree.
			src: `export function fib(n: number): number {
  if (n < 2) {
    return n;
  }
  return fib(n - 1) + fib(n - 2);
}`,
		},
		{
			name:   "concat",
			golden: "func_concat.golden",
			// string is value.BStr, a string literal is value.FromGoString, and +
			// on two strings is value.Concat, not a Go + which would be UTF-8.
			src: `export function greet(name: string): string { return "Hello, " + name; }`,
		},
		{
			name:   "strlen",
			golden: "func_strlen.golden",
			// .length on a string is the UTF-16 code-unit count, the value.BStr
			// Length method, a float64 that matches the number type .length has.
			src: `export function width(a: string, b: string): number { return (a + b).length; }`,
		},
		{
			name:   "streq",
			golden: "func_streq.golden",
			// === on two strings compares by UTF-16 code unit, the value.BStr
			// Equal method, not a Go == on the struct, which would compare
			// backing fields and misjudge two equal strings backed differently.
			src: `export function same(a: string, b: string): boolean { return a === b; }`,
		},
		{
			name:   "strneq",
			golden: "func_strneq.golden",
			// !== is the negation of the same Equal call, so it lowers to a Go
			// unary not over value.BStr.Equal rather than a struct !=.
			src: `export function diff(a: string, b: string): boolean { return a !== b; }`,
		},
		{
			name:   "modulo",
			golden: "func_modulo.golden",
			// % on numbers is fmod, not Go's integer remainder, so it lowers to a
			// math.Mod call rather than a Go % operator.
			src: "export function rem(a: number, b: number): number { return a % b; }",
		},
		{
			name:   "logical",
			golden: "func_logical.golden",
			// && and || on boolean operands map to Go's short-circuit operators,
			// so a compound range check lowers to one Go condition.
			src: `export function between(x: number, lo: number, hi: number): number {
  if (x >= lo && x <= hi) {
    return 1;
  }
  return 0;
}`,
		},
		{
			name:   "for_loop",
			golden: "func_for.golden",
			// a C-style for becomes a Go block holding the let declaration and a
			// for with an empty init, so the loop variable keeps its float64
			// type; the return negates with a unary minus.
			src: `export function negsum(n: number): number {
  let t = 0;
  for (let i = 1; i <= n; i = i + 1) {
    t = t + i;
  }
  return -t;
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
		// a truthy number condition needs JavaScript coercion, not a Go bool.
		{"truthyCond", "export function t(a: number): number { if (a) { return 1; } return 0; }"},
		// a for-of over an iterable is a later slice.
		{"forOf", "export function s(xs: number[]): number { let t = 0; for (const x of xs) { t = t + x; } return t; }"},
		// prefix increment mutates its operand, a later slice.
		{"prefixIncr", "export function p(a: number): number { let b = a; ++b; return b; }"},
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
