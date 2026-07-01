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
			name:   "strlit_escapes",
			golden: "func_strlit_escapes.golden",
			// a literal whose escapes decode to a valid Go string becomes
			// value.FromGoString of the decoded text, so \n and \t are real control
			// characters and \u{1F600} is the emoji in the emitted Go literal.
			src: `export function s(): string { return "tab\tand\nemoji \u{1F600}"; }`,
		},
		{
			name:   "strlit_lone_surrogate",
			golden: "func_strlit_lone_surrogate.golden",
			// a literal whose \u escape names a lone surrogate cannot be a Go string,
			// so it lowers to value.FromUTF16 of the raw code units.
			src: `export function s(): string { return "\uD83D"; }`,
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
			name:   "charcode",
			golden: "func_charcode.golden",
			// s.charCodeAt(i) is a method call on a string receiver, so it lowers
			// to the value.BStr CharCodeAt method rather than a plain function
			// call or an index expression.
			src: `export function codeAt(s: string, i: number): number { return s.charCodeAt(i); }`,
		},
		{
			name:   "charat",
			golden: "func_charat.golden",
			// s.charAt(i) is a string-returning string method, so it lowers to the
			// value.BStr CharAt method and the whole function returns value.BStr.
			src: `export function at(s: string, i: number): string { return s.charAt(i); }`,
		},
		{
			name:   "indexof",
			golden: "func_indexof.golden",
			// a string method that takes a string argument, so the argument-kind
			// guard admits it where the number-arg methods would reject it.
			src: `export function find(s: string, sub: string): number { return s.indexOf(sub); }`,
		},
		{
			name:   "indexof_pos",
			golden: "func_indexof_pos.golden",
			// indexOf with the optional start position, which lowers to the variadic
			// IndexOf with a second argument the source supplied.
			src: `export function findFrom(s: string, sub: string, from: number): number { return s.indexOf(sub, from); }`,
		},
		{
			name:   "lastindexof",
			golden: "func_lastindexof.golden",
			// lastIndexOf takes the same string-then-optional-number shape as indexOf.
			src: `export function findLast(s: string, sub: string): number { return s.lastIndexOf(sub); }`,
		},
		{
			name:   "includes",
			golden: "func_includes.golden",
			// a string-arg method returning a boolean, so the whole function is
			// bool-typed.
			src: `export function has(s: string, sub: string): boolean { return s.includes(sub); }`,
		},
		{
			name:   "startswith",
			golden: "func_startswith.golden",
			src:    `export function starts(s: string, p: string): boolean { return s.startsWith(p); }`,
		},
		{
			name:   "endswith",
			golden: "func_endswith.golden",
			// the suffix companion of startsWith, the same string-then-optional-number
			// shape.
			src: `export function ends(s: string, p: string): boolean { return s.endsWith(p); }`,
		},
		{
			name:   "startswith_pos",
			golden: "func_startswith_pos.golden",
			// startsWith with the optional position, which lowers to the variadic
			// StartsWith with the second argument the source supplied.
			src: `export function startsAt(s: string, p: string, at: number): boolean { return s.startsWith(p, at); }`,
		},
		{
			name:   "slice2",
			golden: "func_slice2.golden",
			// slice with both arguments; the Go method is variadic but the call
			// passes exactly the two the source wrote.
			src: `export function sl(s: string, a: number, b: number): string { return s.slice(a, b); }`,
		},
		{
			name:   "slice1",
			golden: "func_slice1.golden",
			// slice with one argument exercises the optional-argument arity: the
			// same variadic method, called with a single index.
			src: `export function tail(s: string, a: number): string { return s.slice(a); }`,
		},
		{
			name:   "substring",
			golden: "func_substring.golden",
			src:    `export function sub(s: string, a: number, b: number): string { return s.substring(a, b); }`,
		},
		{
			name:   "padstart2",
			golden: "func_padstart2.golden",
			// padStart with both a number and a string argument exercises the
			// mixed argument-kind guard: arg 0 must be a number and arg 1 a string,
			// where the number-only methods would reject the string.
			src: `export function pad(s: string, n: number, p: string): string { return s.padStart(n, p); }`,
		},
		{
			name:   "padstart1",
			golden: "func_padstart1.golden",
			// padStart with only the length argument exercises the optional pad
			// string: the same variadic Go method, called without the pad, which
			// then defaults to a space.
			src: `export function padsp(s: string, n: number): string { return s.padStart(n); }`,
		},
		{
			name:   "padend2",
			golden: "func_padend2.golden",
			src:    `export function padr(s: string, n: number, p: string): string { return s.padEnd(n, p); }`,
		},
		{
			name:   "concat_method",
			golden: "func_concat_method.golden",
			// s.concat(a, b) is the variadic concat method, not the + operator, so it
			// lowers to the value.BStr ConcatN method with every argument passed
			// through; the variadic arity admits the two string arguments.
			src: `export function join(s: string, a: string, b: string): string { return s.concat(a, b); }`,
		},
		{
			name:   "math_floor",
			golden: "func_math_floor.golden",
			// Math.floor is a call on the global Math namespace, so the receiver is
			// not lowered to a value; it becomes the math package qualifier and the
			// call is math.Floor.
			src: `export function fl(x: number): number { return Math.floor(x); }`,
		},
		{
			name:   "math_pow",
			golden: "func_math_pow.golden",
			// a two-argument Math method lowers to a two-argument math function.
			src: `export function power(a: number, b: number): number { return Math.pow(a, b); }`,
		},
		{
			name:   "math_max",
			golden: "func_math_max.golden",
			// Math.max takes any number of arguments, so it lowers to the variadic
			// value.MaxN rather than the two-argument math.Max.
			src: `export function bigger(a: number, b: number): number { return Math.max(a, b); }`,
		},
		{
			name:   "math_min3",
			golden: "func_math_min3.golden",
			// three arguments, which the old two-argument arity would have handed
			// back; value.MinN takes the whole list.
			src: `export function least(a: number, b: number, c: number): number { return Math.min(a, b, c); }`,
		},
		{
			name:   "math_nested",
			golden: "func_math_nested.golden",
			// Math calls compose: the argument to one is the result of another, so
			// the whole expression lowers to nested math calls.
			src: `export function d(a: number, b: number): number { return Math.sqrt(Math.abs(a - b)); }`,
		},
		{
			name:   "math_round",
			golden: "func_math_round.golden",
			// Math.round does not lower to math.Round: JavaScript breaks a tie toward
			// +Infinity, so the call goes to value.Round which carries that rule.
			src: `export function rnd(x: number): number { return Math.round(x); }`,
		},
		{
			name:   "math_sign",
			golden: "func_math_sign.golden",
			// Go has no math.Sign, so Math.sign lowers to value.Sign.
			src: `export function sgn(x: number): number { return Math.sign(x); }`,
		},
		{
			name:   "num_hex",
			golden: "func_num_hex.golden",
			// a hexadecimal integer literal is a number like any other, so it lowers to
			// the same hex spelling as a Go int constant, added as a float64.
			src: `export function mask(x: number): number { return x + 0xFF; }`,
		},
		{
			name:   "num_separators",
			golden: "func_num_separators.golden",
			// underscore digit separators are stripped, so the emitted Go literal is
			// the same value with no separators.
			src: `export function big(): number { return 1_000_000; }`,
		},
		{
			name:   "num_exponent",
			golden: "func_num_exponent.golden",
			// an exponent literal carries the .eE that marks it a Go float constant, so
			// it stays a float literal in the emitted code.
			src: `export function milli(): number { return 1.5e-3; }`,
		},
		{
			name:   "bit_and",
			golden: "func_bit_and.golden",
			// & on numbers is not a Go & on float64; each operand coerces through
			// value.ToInt32, the operator runs on the ints, and the result casts
			// back to float64.
			src: `export function band(a: number, b: number): number { return a & b; }`,
		},
		{
			name:   "bit_or_xor",
			golden: "func_bit_or_xor.golden",
			// | and ^ compose the same way, so a nested expression pins both in one
			// golden.
			src: `export function mix(a: number, b: number, c: number): number { return (a | b) ^ c; }`,
		},
		{
			name:   "bit_shl",
			golden: "func_bit_shl.golden",
			// << masks the shift count to five bits through value.ToUint32, so the
			// right operand lowers differently from the left.
			src: `export function shl(a: number, b: number): number { return a << b; }`,
		},
		{
			name:   "bit_shr",
			golden: "func_bit_shr.golden",
			// >> is an arithmetic shift, carried by the signed ToInt32 left operand.
			src: `export function shr(a: number, b: number): number { return a >> b; }`,
		},
		{
			name:   "bit_ushr",
			golden: "func_bit_ushr.golden",
			// >>> is a logical shift: the left operand coerces with ToUint32 so Go's
			// >> on an unsigned type is logical and the result is non-negative.
			src: `export function ushr(a: number, b: number): number { return a >>> b; }`,
		},
		{
			name:   "number_isinteger",
			golden: "func_number_isinteger.golden",
			// Number.isInteger is a static call on the global Number namespace, so
			// it lowers to the value.NumberIsInteger predicate and the function
			// returns bool.
			src: `export function isInt(x: number): boolean { return Number.isInteger(x); }`,
		},
		{
			name:   "number_isnan",
			golden: "func_number_isnan.golden",
			src:    `export function nan(x: number): boolean { return Number.isNaN(x); }`,
		},
		{
			name:   "global_isnan",
			golden: "func_global_isnan.golden",
			// the bare global isNaN, not the Number static one, so the callee is a
			// plain identifier. On a number argument it coerces to nothing, so it
			// lowers to the same value.NumberIsNaN predicate.
			src: `export function nan(x: number): boolean { return isNaN(x); }`,
		},
		{
			name:   "global_isfinite",
			golden: "func_global_isfinite.golden",
			// the bare global isFinite lowers to value.NumberIsFinite the same way.
			src: `export function fin(x: number): boolean { return isFinite(x); }`,
		},
		{
			name:   "bit_not",
			golden: "func_bit_not.golden",
			// ~ is the unary bitwise operator, so it uses the same ToInt32 coercion
			// as the binary bitwise operators: float64(^value.ToInt32(x)), not a Go
			// ^ on the float64.
			src: `export function inv(x: number): number { return ~x; }`,
		},
		{
			name:   "trim",
			golden: "func_trim.golden",
			// a zero-argument string method, so the parameter list is empty and
			// the call takes no arguments.
			src: `export function clean(s: string): string { return s.trim(); }`,
		},
		{
			name:   "trimstart",
			golden: "func_trimstart.golden",
			src:    `export function lead(s: string): string { return s.trimStart(); }`,
		},
		{
			name:   "trimend",
			golden: "func_trimend.golden",
			src:    `export function tailws(s: string): string { return s.trimEnd(); }`,
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
		// a locally shadowed Math is a value receiver, not the global namespace, so
		// its method must not lower to the Go math package; it hands back as an
		// unlowered non-string receiver instead of silently becoming math.Floor.
		{"shadowedMath", "export function m(x: number): number { const Math = { floor: (n: number): number => n }; return Math.floor(x); }"},
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
