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
// A case passes only when both produce the same result for the same arguments,
// so a mistranslation in the mapping table shows up as a diverging result rather
// than passing silently. This is the "TypeScript and generated Go are identical"
// guarantee the directive asks for, mechanized.
//
// Arguments and results are typed, not just numbers: a tuple element is a Go
// float64, string, or bool, and the case names the return kind, so a function
// that takes and returns strings is exercised with real strings and its UTF-16
// value.BStr result is compared against the engine's JavaScript string. Both
// sides reduce their result to one canonical string, and the case passes when
// those strings are equal.

// equivCase is one function exercised with several argument tuples. The source
// defines the exported functions; fn is the one the calls drive. Each tuple
// element is a float64, string, or bool. ret names the return kind ("number",
// "string", or "boolean"); an empty ret means number, the common case.
type equivCase struct {
	name string
	src  string
	fn   string
	ret  string
	args [][]any
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
			args: [][]any{{0}, {42}, {-7}, {3.5}},
		},
		{
			name: "add",
			src:  "export function add(a: number, b: number): number { return a + b; }",
			fn:   "add",
			args: [][]any{{1, 2}, {-3, 3}, {0.1, 0.2}, {1e6, 1}},
		},
		{
			name: "arithmetic",
			src:  "export function mix(a: number, b: number): number { return (a + b) * a - b / 2; }",
			fn:   "mix",
			args: [][]any{{2, 4}, {5, 10}, {-1, -2}, {1.5, 0.5}},
		},
		{
			name: "keywordParam",
			src:  "export function pick(type: number): number { return type; }",
			fn:   "pick",
			args: [][]any{{9}, {-4}},
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
			args: [][]any{{0}, {1}, {3}, {5}, {10}},
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
			args: [][]any{{0}, {1}, {5}, {10}},
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
			args: [][]any{{-5}, {0}, {42}, {100}, {250}},
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
			args: [][]any{{0}, {1}, {7}, {12}},
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
			args: [][]any{{0}, {4}, {10}},
		},
		{
			name: "stringLength",
			// The literal holds an astral character (one rune, two UTF-16 code
			// units). .length must report code units, so the whole string is
			// "a" (1) + emoji (2) + "b" (1) = 4. A Go len() over UTF-8 bytes would
			// say 6, so this proves the emitted Go runs the real value.BStr and
			// gets the JavaScript answer, matched against quickjs.
			src: `export function width(): number {
  let s = "a" + "😀" + "b";
  return s.length;
}`,
			fn:   "width",
			args: [][]any{{}},
		},
		{
			name: "greet",
			// A string argument in and a string result out: concatenation with a
			// real value, including an astral emoji, must round-trip through the
			// value model and read back identically to what quickjs produced.
			src:  `export function greet(name: string): string { return "Hello, " + name; }`,
			fn:   "greet",
			ret:  "string",
			args: [][]any{{"World"}, {"😀"}, {""}, {"a b c"}},
		},
		{
			name: "stringEq",
			// === on two strings compares by UTF-16 code unit and returns a real
			// boolean, so the case pairs equal, unequal, empty, and astral inputs.
			src:  `export function same(a: string, b: string): boolean { return a === b; }`,
			fn:   "same",
			ret:  "boolean",
			args: [][]any{{"x", "x"}, {"x", "y"}, {"", ""}, {"😀", "😀"}, {"😀", "😁"}},
		},
		{
			name: "stringNeq",
			// !== is the negation, exercised over the same shapes so the emitted
			// !value.Equal matches JavaScript's inequality point for point.
			src:  `export function diff(a: string, b: string): boolean { return a !== b; }`,
			fn:   "diff",
			ret:  "boolean",
			args: [][]any{{"x", "x"}, {"x", "y"}, {"", ""}, {"😀", "😀"}},
		},
		{
			name: "charCodeAt",
			// A string in and a number out through a method call. The cases cover
			// in-range indices, an out-of-range index and a negative one (both NaN
			// in JavaScript, which the value model returns and the harness compares
			// as "NaN" on both sides), and the two surrogate halves of an astral
			// character, where charCodeAt(0) and charCodeAt(1) are the high and low
			// code units, not one code point.
			src:  `export function codeAt(s: string, i: number): number { return s.charCodeAt(i); }`,
			fn:   "codeAt",
			args: [][]any{{"abc", 0}, {"abc", 1}, {"abc", 2}, {"abc", 3}, {"abc", -1}, {"😀", 0}, {"😀", 1}},
		},
		{
			name: "charAt",
			// A string in and a string out through a method call. In-range indices
			// return the one-character string, and out-of-range and negative
			// indices return the empty string, which JavaScript charAt does and
			// value.BStr.CharAt matches. The receiver is a BMP string so every
			// result round-trips through UTF-8 cleanly; the lone-surrogate case an
			// astral receiver produces is pinned in the value package unit test
			// instead, where it is checked code unit for code unit rather than
			// through a lossy string comparison.
			src:  `export function at(s: string, i: number): string { return s.charAt(i); }`,
			fn:   "at",
			ret:  "string",
			args: [][]any{{"abc", 0}, {"abc", 1}, {"abc", 2}, {"abc", 3}, {"abc", -1}},
		},
		{
			name: "indexOf",
			// Two strings in, a number out, through a method that takes a string
			// argument. The cases cover a hit at the start, in the middle, a miss
			// (-1), the empty search (0), and a search inside an astral character
			// so the code-unit index matches JavaScript rather than a rune index.
			src:  `export function find(s: string, sub: string): number { return s.indexOf(sub); }`,
			fn:   "find",
			args: [][]any{{"hello", "he"}, {"hello", "lo"}, {"hello", "z"}, {"hello", ""}, {"a😀b", "b"}},
		},
		{
			name: "indexOfFrom",
			// indexOf with the start position: a match before the position is skipped,
			// a match exactly at it counts, a position past the end gives -1 for a
			// non-empty search and the length for the empty search.
			src:  `export function findFrom(s: string, sub: string, from: number): number { return s.indexOf(sub, from); }`,
			fn:   "findFrom",
			args: [][]any{{"abcabc", "a", 1}, {"abcabc", "c", 2}, {"abcabc", "a", 4}, {"abc", "", 99}, {"abc", "a", -5}},
		},
		{
			name: "lastIndexOf",
			// lastIndexOf reports the greatest matching index, so it differs from
			// indexOf on a string with two matches; the cases cover the two-match
			// string, a miss, the empty search, and an astral character.
			src:  `export function findLast(s: string, sub: string): number { return s.lastIndexOf(sub); }`,
			fn:   "findLast",
			args: [][]any{{"abcabc", "a"}, {"abcabc", "c"}, {"hello", "z"}, {"hello", ""}, {"a😀b", "b"}},
		},
		{
			name: "includes",
			// The boolean companion of indexOf over the same shapes.
			src:  `export function has(s: string, sub: string): boolean { return s.includes(sub); }`,
			fn:   "has",
			ret:  "boolean",
			args: [][]any{{"hello", "ell"}, {"hello", "z"}, {"hello", ""}, {"a😀b", "😀"}},
		},
		{
			name: "startsWith",
			src:  `export function starts(s: string, p: string): boolean { return s.startsWith(p); }`,
			fn:   "starts",
			ret:  "boolean",
			args: [][]any{{"hello", "he"}, {"hello", "lo"}, {"hello", ""}, {"hi", "hello"}, {"😀x", "😀"}},
		},
		{
			name: "endsWith",
			// the suffix companion, covering a real suffix, a non-suffix, the empty
			// suffix, a suffix longer than the string, and an astral suffix.
			src:  `export function ends(s: string, p: string): boolean { return s.endsWith(p); }`,
			fn:   "ends",
			ret:  "boolean",
			args: [][]any{{"hello", "lo"}, {"hello", "he"}, {"hello", ""}, {"hi", "hello"}, {"x😀", "😀"}},
		},
		{
			name: "startsWithPos",
			// startsWith with the position: the prefix must begin at it, so a prefix
			// that matches at 0 fails at a later position and matches at its own.
			src:  `export function startsAt(s: string, p: string, at: number): boolean { return s.startsWith(p, at); }`,
			fn:   "startsAt",
			ret:  "boolean",
			args: [][]any{{"abcabc", "abc", 3}, {"abcabc", "abc", 1}, {"abcabc", "bc", 1}, {"abc", "", 99}},
		},
		{
			name: "endsWithPos",
			// endsWith with the end position: the window ends there, so a prefix of the
			// string reads as a suffix of the shortened window.
			src:  `export function endsAt(s: string, p: string, at: number): boolean { return s.endsWith(p, at); }`,
			fn:   "endsAt",
			ret:  "boolean",
			args: [][]any{{"hello", "hell", 4}, {"hello", "lo", 4}, {"hello", "hel", 3}},
		},
		{
			name: "slice2",
			// slice with both bounds over a BMP receiver, covering an interior
			// range, the full string, negative-from-end bounds, an empty result
			// when start is at or past end, and an end past the length.
			src:  `export function sl(s: string, a: number, b: number): string { return s.slice(a, b); }`,
			fn:   "sl",
			ret:  "string",
			args: [][]any{{"hello", 1, 3}, {"hello", 0, 5}, {"hello", -3, -1}, {"hello", 3, 1}, {"hello", 2, 100}},
		},
		{
			name: "slice1",
			// slice with a single bound proves the optional-argument arity end to
			// end: a positive start, a negative start, and a start past the end.
			src:  `export function tail(s: string, a: number): string { return s.slice(a); }`,
			fn:   "tail",
			ret:  "string",
			args: [][]any{{"hello", 0}, {"hello", 2}, {"hello", -2}, {"hello", 10}},
		},
		{
			name: "substring",
			// substring differs from slice at the edges: negatives become 0 and a
			// start past end swaps, so the cases pin both behaviors against the
			// engine.
			src:  `export function sub(s: string, a: number, b: number): string { return s.substring(a, b); }`,
			fn:   "sub",
			ret:  "string",
			args: [][]any{{"hello", 1, 3}, {"hello", 3, 1}, {"hello", -2, 3}, {"hello", 2, 100}},
		},
		{
			name: "padStart2",
			// padStart with an explicit pad string: a target longer than the
			// string pads and repeats-then-truncates the pad, a target not longer
			// leaves the string alone, and an empty pad string is a no-op.
			src:  `export function pad(s: string, n: number, p: string): string { return s.padStart(n, p); }`,
			fn:   "pad",
			ret:  "string",
			args: [][]any{{"5", 3, "0"}, {"5", 1, "0"}, {"5", 6, "ab"}, {"abc", 5, ""}, {"abc", -2, "0"}},
		},
		{
			name: "padStart1",
			// padStart with no pad argument defaults to a space, the optional-arg
			// path over a mixed number-then-string signature.
			src:  `export function padsp(s: string, n: number): string { return s.padStart(n); }`,
			fn:   "padsp",
			ret:  "string",
			args: [][]any{{"7", 4}, {"7", 1}, {"hi", 5}},
		},
		{
			name: "padEnd2",
			// padEnd appends the filler instead of prepending it; same rules.
			src:  `export function padr(s: string, n: number, p: string): string { return s.padEnd(n, p); }`,
			fn:   "padr",
			ret:  "string",
			args: [][]any{{"5", 3, "0"}, {"5", 6, "ab"}, {"abc", 2, "0"}, {"abc", 5, ""}},
		},
		{
			name: "mathFloor",
			// Math.floor over positive, negative, and already-integer inputs, where
			// floor differs from truncation on the negative fraction.
			src:  `export function fl(x: number): number { return Math.floor(x); }`,
			fn:   "fl",
			args: [][]any{{3.7}, {-3.2}, {5}, {-0.5}},
		},
		{
			name: "mathCeilTruncAbs",
			// three one-argument methods composed, so the whole chain is checked at
			// once: ceil then abs of a trunc, covering the sign handling each does.
			src:  `export function c(x: number): number { return Math.ceil(Math.abs(Math.trunc(x))); }`,
			fn:   "c",
			args: [][]any{{3.9}, {-3.9}, {0}},
		},
		{
			name: "mathSqrt",
			// perfect squares so the result is exact on both sides, since IEEE sqrt
			// is correctly rounded and deterministic.
			src:  `export function r(x: number): number { return Math.sqrt(x); }`,
			fn:   "r",
			args: [][]any{{4}, {9}, {2}, {0}},
		},
		{
			name: "mathPow",
			// integer and power-of-two exponents, whose results are exact so the two
			// pow implementations cannot diverge in the last bit.
			src:  `export function p(a: number, b: number): number { return Math.pow(a, b); }`,
			fn:   "p",
			args: [][]any{{2, 10}, {5, 3}, {2, -2}, {9, 0}},
		},
		{
			name: "mathMinMax",
			// min and max fold two numbers; the cases cover the ordinary order, the
			// reversed order, and equal values.
			src:  `export function mm(a: number, b: number): number { return Math.max(a, Math.min(a, b)); }`,
			fn:   "mm",
			args: [][]any{{3, 7}, {7, 3}, {4, 4}, {-1, -5}},
		},
		{
			name: "mathMin3",
			// three arguments through the variadic Math.min, which the two-argument
			// lowering could not do; the cases put the smallest first, middle, and
			// last so the fold order does not matter.
			src:  `export function m3(a: number, b: number, c: number): number { return Math.min(a, b, c); }`,
			fn:   "m3",
			args: [][]any{{1, 2, 3}, {2, 1, 3}, {3, 2, 1}, {5, 5, 5}},
		},
		{
			name: "mathRound",
			// the half-way inputs are where JavaScript and Go's math.Round disagree:
			// 2.5 rounds to 3 both ways, but -2.5 rounds to -2 in JavaScript (toward
			// +Infinity) and -3 in Go, so this case fails unless value.Round is used.
			src:  `export function rnd(x: number): number { return Math.round(x); }`,
			fn:   "rnd",
			args: [][]any{{2.5}, {-2.5}, {2.4}, {-2.6}, {0}},
		},
		{
			name: "mathSign",
			// sign over positive, negative, and zero; the zero passes through so both
			// sides print the same, and the nonzero cases give the constant one.
			src:  `export function sgn(x: number): number { return Math.sign(x); }`,
			fn:   "sgn",
			args: [][]any{{5}, {-5}, {0}, {0.001}, {-0.001}},
		},
		{
			name: "bitAndOrXor",
			// the three logical bitwise operators over positive, negative (which
			// exercises the two's-complement ToInt32 wrap), and zero operands.
			src:  `export function b(a: number, c: number): number { return (a & c) | (a ^ c); }`,
			fn:   "b",
			args: [][]any{{12, 10}, {-1, 255}, {0, 0}, {-8, 3}},
		},
		{
			name: "bitShifts",
			// left and arithmetic-right shift, including a negative left operand so
			// the sign propagation of >> is checked, and a count above 31 to pin the
			// five-bit mask.
			src:  `export function sh(a: number, n: number): number { return (a << n) >> n; }`,
			fn:   "sh",
			args: [][]any{{1, 4}, {-1, 2}, {255, 1}, {1, 33}},
		},
		{
			name: "bitUnsignedShift",
			// >>> is the operator that most visibly differs from a signed shift:
			// -1 >>> 0 is 4294967295, not -1, so the ToUint32 coercion is what makes
			// the emitted Go agree with the engine.
			src:  `export function us(a: number, n: number): number { return a >>> n; }`,
			fn:   "us",
			args: [][]any{{-1, 0}, {-1, 1}, {256, 2}, {8, 1}},
		},
		{
			name: "bitNot",
			// ~x is -(x+1) on the coerced integer, so the cases cover positive,
			// negative, zero, and a fraction that must truncate first.
			src:  `export function inv(x: number): number { return ~x; }`,
			fn:   "inv",
			args: [][]any{{0}, {5}, {-1}, {6.9}, {-6.9}},
		},
		{
			name: "bitCoerceFraction",
			// a fractional operand must truncate before the bitwise op, the ToInt32
			// step, so 6.9 & 3 is 6 & 3, not a float operation.
			src:  `export function f(a: number, c: number): number { return a & c; }`,
			fn:   "f",
			args: [][]any{{6.9, 3}, {-6.9, 3}},
		},
		{
			name: "numberIsInteger",
			// the argument is x / y so the cases can reach a fraction (7/2), a whole
			// number (6/2), and a non-finite value (1/0 is Infinity), all of which
			// Number.isInteger must judge the same way the engine does.
			src:  `export function ii(x: number, y: number): boolean { return Number.isInteger(x / y); }`,
			fn:   "ii",
			ret:  "boolean",
			args: [][]any{{6, 2}, {7, 2}, {1, 0}, {-4, 2}},
		},
		{
			name: "numberIsFiniteNaN",
			// isFinite and isNaN over the same x / y, covering a finite result, an
			// infinity (1/0), and a NaN (0/0).
			src:  `export function fn(x: number, y: number): boolean { return Number.isFinite(x / y) || Number.isNaN(x / y); }`,
			fn:   "fn",
			ret:  "boolean",
			args: [][]any{{3, 2}, {1, 0}, {0, 0}},
		},
		{
			name: "numberIsSafeInteger",
			// x * y lets a case exceed the safe-integer range: 9007199254740992 is
			// 2^53, an integer that is not safe, so the harness pins the boundary.
			src:  `export function si(x: number, y: number): boolean { return Number.isSafeInteger(x * y); }`,
			fn:   "si",
			ret:  "boolean",
			args: [][]any{{2, 3}, {4503599627370496, 2}, {4503599627370497, 2}},
		},
		{
			name: "trim",
			// The inputs carry the exact ECMAScript whitespace set, not just ASCII
			// spaces: a tab and newlines, a no-break space (U+00A0), and a
			// zero-width no-break space (U+FEFF), all of which trim removes and
			// Go's unicode.IsSpace would get wrong. An all-whitespace string trims
			// to empty and a clean string is unchanged.
			src:  `export function clean(s: string): string { return s.trim(); }`,
			fn:   "clean",
			ret:  "string",
			args: [][]any{{"  hello  "}, {"\t\n hi \r\n"}, {"\u00a0x\u00a0"}, {"\ufeffy\ufeff"}, {"none"}, {"   "}},
		},
		{
			name: "trimStart",
			// trimStart removes only the leading run, so trailing whitespace must
			// survive, which the engine and the emitted Go must agree on.
			src:  `export function lead(s: string): string { return s.trimStart(); }`,
			fn:   "lead",
			ret:  "string",
			args: [][]any{{"  hi  "}, {" x"}, {"none"}},
		},
		{
			name: "trimEnd",
			src:  `export function tailws(s: string): string { return s.trimEnd(); }`,
			fn:   "tailws",
			ret:  "string",
			args: [][]any{{"  hi  "}, {"x "}, {"none"}},
		},
		{
			name: "strlitEscapes",
			// a literal carrying control-character escapes, a \x hex escape, and a
			// braced \u{...} code point (the emoji): the compiler decodes them to code
			// units at lower time, and the engine decodes them at parse time, so the
			// two returned strings must match. A zero-argument function, so the one
			// call passes no arguments.
			src:  `export function lit(): string { return "tab\tnl\nhex\x41 emoji\u{1F600}"; }`,
			fn:   "lit",
			ret:  "string",
			args: [][]any{{}},
		},
		{
			name: "numLiterals",
			// the numeric-literal forms this slice lowers: hex, binary, and octal
			// integers, an underscore-separated decimal, and an exponent. The compiler
			// emits each as the Go literal for the same value and the engine parses the
			// same source, so their sum must agree. A zero-argument function.
			src:  `export function n(): number { return 0xFF + 0b1010 + 0o17 + 1_000 + 1.5e2; }`,
			fn:   "n",
			args: [][]any{{}},
		},
		{
			name: "modulo",
			src:  "export function rem(a: number, b: number): number { return a % b; }",
			// fmod keeps the sign of the dividend and works on fractions, so the
			// cases cover negative dividends and a non-integer operand, where a
			// naive integer remainder would diverge from JavaScript.
			fn:   "rem",
			args: [][]any{{7, 3}, {-7, 3}, {7, -3}, {5.5, 2}, {10, 10}},
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
			args: [][]any{{5, 0, 10}, {-1, 0, 10}, {11, 0, 10}, {0, 0, 10}, {10, 0, 10}},
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
			args: [][]any{{3, 4}, {0, 0}, {-2, 5}},
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
				want := evalTS(t, tc.src, tc.fn, tc.ret, args)
				got := runGo(t, goSrc, imports, goName, tc.ret, args)
				if got != want {
					t.Errorf("%s(%v): generated Go = %s, TypeScript = %s", tc.fn, args, got, want)
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
// and calls the function with the given arguments, returning the canonical form
// of the result. This is the reference: the source's own runtime meaning.
func evalTS(t *testing.T, src, fn, ret string, args []any) string {
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
		callArgs[i] = engineArg(a)
	}
	res, err := eng.Call(fn, callArgs...)
	if err != nil {
		t.Fatalf("Call %s: %v", fn, err)
	}
	return canonResult(t, ret, res)
}

// runGo wraps the generated function in a tiny main, compiles and runs it with
// the Go toolchain, and returns the canonical form of what it printed. This is
// the subject: what the emitted Go actually computes, not what we hope it does.
//
// The temporary package is created inside this repository's tree rather than in
// an isolated temp module, so it compiles under bento's own go.mod and go.sum
// and links the real value package with no separate require, replace, or module
// download. That keeps the test fully offline: a throwaway module would need its
// own go.sum for the runtime import, which a clean CI checkout does not have.
func runGo(t *testing.T, goSrc string, imports []string, name, ret string, args []any) string {
	t.Helper()
	dir, err := os.MkdirTemp(repoRoot(t), "eqrun-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	callArgs := make([]string, len(args))
	for i, a := range args {
		callArgs[i] = goArg(a)
	}
	// The wrapper always prints with fmt; the lowered code adds whatever else it
	// referenced (math for a % that became math.Mod, and the value model for a
	// string). A string argument names value.FromGoString, but the value import
	// is already present whenever a string crosses the signature, so no extra
	// import is needed here.
	paths := append([]string{"fmt"}, imports...)
	var imp strings.Builder
	imp.WriteString("import (\n")
	for _, p := range paths {
		imp.WriteString("\t\"" + p + "\"\n")
	}
	imp.WriteString(")")
	call := fmt.Sprintf("%s(%s)", name, strings.Join(callArgs, ", "))
	main := fmt.Sprintf(
		"package main\n\n%s\n\n%s\nfunc main() {\n\t%s\n}\n",
		imp.String(), goSrc, goPrint(ret, call),
	)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go run failed: %v\n--- program ---\n%s\n--- output ---\n%s", err, main, out)
	}
	return canonOutput(t, ret, strings.TrimSpace(string(out)))
}

// engineArg converts a case argument to the value the engine wants. Numbers in
// the case tables are written as untyped Go constants (an int like 42), so they
// are widened to float64 here to match JavaScript's single number type; strings
// and booleans pass through.
func engineArg(a any) any {
	switch v := a.(type) {
	case int:
		return float64(v)
	default:
		return v
	}
}

// goArg renders a case argument as the Go expression that reproduces it in the
// generated program. A number is a numeric constant (untyped, so it takes the
// float64 parameter type), a string is a value.BStr built from a Go literal, and
// a boolean is a Go boolean literal.
func goArg(a any) string {
	switch v := a.(type) {
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case string:
		return "value.FromGoString(" + strconv.Quote(v) + ")"
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// goPrint returns the statement that prints the call result in the wrapper,
// chosen by return kind: a number prints with %g, a string prints its Go-quoted
// UTF-8 form after decoding the value.BStr, and a boolean prints true or false.
func goPrint(ret, call string) string {
	switch ret {
	case "string":
		return fmt.Sprintf("fmt.Printf(\"%%q\\n\", (%s).ToGoString())", call)
	case "boolean":
		return fmt.Sprintf("fmt.Printf(\"%%t\\n\", %s)", call)
	default:
		return fmt.Sprintf("fmt.Printf(\"%%g\\n\", %s)", call)
	}
}

// canonResult reduces the engine's native return value to the canonical string
// the two sides compare on, using the case's declared return kind so a number, a
// string, and a boolean each get one unambiguous spelling.
func canonResult(t *testing.T, ret string, res any) string {
	t.Helper()
	switch ret {
	case "string":
		s, ok := res.(string)
		if !ok {
			t.Fatalf("engine returned %T (%v), want a string", res, res)
		}
		return strconv.Quote(s)
	case "boolean":
		b, ok := res.(bool)
		if !ok {
			t.Fatalf("engine returned %T (%v), want a boolean", res, res)
		}
		return strconv.FormatBool(b)
	default:
		f, ok := toFloat(res)
		if !ok {
			t.Fatalf("engine returned %T (%v), want a number", res, res)
		}
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
}

// canonOutput reduces the generated program's printed line to the same canonical
// form canonResult produces, so the two are compared as equal strings. The
// string and boolean prints are already canonical; a number is reparsed and
// reformatted so its spelling matches the engine side exactly.
func canonOutput(t *testing.T, ret, out string) string {
	t.Helper()
	switch ret {
	case "string", "boolean":
		return out
	default:
		f, err := strconv.ParseFloat(out, 64)
		if err != nil {
			t.Fatalf("parse go output %q: %v", out, err)
		}
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
}

// repoRoot returns the absolute path of the repository root. The test runs with
// its working directory at the package (pkg/lower), so the root is two levels
// up. The generated program is placed under this root so it builds inside the
// bento module and can import the runtime with no separate module wiring.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join(wd, "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
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
