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
// lives in a checked-in testdata/<file>.ts, read at run time so the exact
// TypeScript being compared is a file a reviewer can open; fn is the exported
// function the calls drive. Each tuple element is a float64, string, or bool.
// ret names the return kind ("number", "string", or "boolean"); an empty ret
// means number, the common case.
type equivCase struct {
	name string
	file string
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
			file: "eq_identity",
			fn:   "identity",
			args: [][]any{{0}, {42}, {-7}, {3.5}},
		},
		{
			name: "add",
			file: "eq_add",
			fn:   "add",
			args: [][]any{{1, 2}, {-3, 3}, {0.1, 0.2}, {1e6, 1}},
		},
		{
			name: "arithmetic",
			file: "eq_arithmetic",
			fn:   "mix",
			args: [][]any{{2, 4}, {5, 10}, {-1, -2}, {1.5, 0.5}},
		},
		{
			name: "keywordParam",
			file: "eq_keywordParam",
			fn:   "pick",
			args: [][]any{{9}, {-4}},
		},
		{
			name: "loop",
			file: "eq_loop",
			fn:   "score",
			args: [][]any{{0}, {1}, {3}, {5}, {10}},
		},
		{
			name: "factorial",
			file: "eq_factorial",
			fn:   "fact",
			args: [][]any{{0}, {1}, {5}, {10}},
		},
		{
			name: "branch",
			file: "eq_branch",
			fn:   "clamp",
			args: [][]any{{-5}, {0}, {42}, {100}, {250}},
		},
		{
			name: "recursion",
			file: "eq_recursion",
			fn:   "fib",
			args: [][]any{{0}, {1}, {7}, {12}},
		},
		{
			name: "forLoopNegate",
			file: "eq_forLoopNegate",
			fn:   "negsum",
			args: [][]any{{0}, {4}, {10}},
		},
		{
			// i++ in the post clause plus +=, -=, and *= as statements.
			name: "compoundArith",
			file: "eq_compound_arith",
			fn:   "accumulate",
			args: [][]any{{0}, {1}, {5}, {10}},
		},
		{
			// += on strings must concatenate as UTF-16, matched against the engine.
			name: "compoundString",
			file: "eq_compound_string",
			fn:   "join",
			ret:  "string",
			args: [][]any{{"foo", "bar"}, {"", "x"}, {"a", ""}},
		},
		{
			// %= is fmod, keeping the sign of the dividend, over positive, negative, and fractional operands.
			name: "compoundMod",
			file: "eq_compound_mod",
			fn:   "wrap",
			args: [][]any{{7, 3}, {-7, 3}, {5.5, 2}},
		},
		{
			// ++ then -- twice nets a decrement, checked over a range.
			name: "incDec",
			file: "eq_incdec",
			fn:   "step",
			args: [][]any{{0}, {5}, {-3}},
		},
		{
			// a chained ternary picks -1, 0, or 1 across the sign boundaries.
			name: "conditional",
			file: "eq_conditional",
			fn:   "sign",
			args: [][]any{{-5}, {0}, {8}},
		},
		{
			// a ternary with string branches, matched against the engine's string.
			name: "conditionalString",
			file: "eq_conditional_string",
			fn:   "label",
			ret:  "string",
			args: [][]any{{5}, {0}, {-2}},
		},
		{
			name: "stringLength",
			// The literal holds an astral character (one rune, two UTF-16 code
			// units). .length must report code units, so the whole string is
			// "a" (1) + emoji (2) + "b" (1) = 4. A Go len() over UTF-8 bytes would
			// say 6, so this proves the emitted Go runs the real value.BStr and
			// gets the JavaScript answer, matched against quickjs.
			file: "eq_stringLength",
			fn:   "width",
			args: [][]any{{}},
		},
		{
			name: "greet",
			// A string argument in and a string result out: concatenation with a
			// real value, including an astral emoji, must round-trip through the
			// value model and read back identically to what quickjs produced.
			file: "eq_greet",
			fn:   "greet",
			ret:  "string",
			args: [][]any{{"World"}, {"😀"}, {""}, {"a b c"}},
		},
		{
			name: "stringEq",
			// === on two strings compares by UTF-16 code unit and returns a real
			// boolean, so the case pairs equal, unequal, empty, and astral inputs.
			file: "eq_stringEq",
			fn:   "same",
			ret:  "boolean",
			args: [][]any{{"x", "x"}, {"x", "y"}, {"", ""}, {"😀", "😀"}, {"😀", "😁"}},
		},
		{
			name: "stringNeq",
			// !== is the negation, exercised over the same shapes so the emitted
			// !value.Equal matches JavaScript's inequality point for point.
			file: "eq_stringNeq",
			fn:   "diff",
			ret:  "boolean",
			args: [][]any{{"x", "x"}, {"x", "y"}, {"", ""}, {"😀", "😀"}},
		},
		{
			name: "stringLess",
			// < orders by code unit; returning the operator result directly makes the
			// case discriminating, so the harness would catch an ordering that merely
			// stayed self-consistent. The inputs cover a straight less-than, its
			// reverse, equal strings, a prefix before its extension, uppercase below
			// lowercase, and the astral case where the leading surrogate D83D orders
			// below the BMP letter z.
			file: "eq_stringLess",
			fn:   "lt",
			ret:  "boolean",
			args: [][]any{{"a", "b"}, {"b", "a"}, {"x", "x"}, {"ab", "abc"}, {"Z", "a"}, {"😀", "z"}},
		},
		{
			name: "stringLessEqual",
			// <= differs from < only on equal strings, so the equal pair is the case
			// that separates them.
			file: "eq_stringLessEqual",
			fn:   "le",
			ret:  "boolean",
			args: [][]any{{"a", "b"}, {"b", "a"}, {"x", "x"}, {"ab", "abc"}},
		},
		{
			name: "stringGreaterEqual",
			// > and >= are the mirror, carried by the same Compare against zero with
			// the flipped token; one case pins both together.
			file: "eq_stringGreaterEqual",
			fn:   "ge",
			ret:  "boolean",
			args: [][]any{{"b", "a"}, {"a", "b"}, {"x", "x"}, {"abc", "ab"}},
		},
		{
			name: "charCodeAt",
			// A string in and a number out through a method call. The cases cover
			// in-range indices, an out-of-range index and a negative one (both NaN
			// in JavaScript, which the value model returns and the harness compares
			// as "NaN" on both sides), and the two surrogate halves of an astral
			// character, where charCodeAt(0) and charCodeAt(1) are the high and low
			// code units, not one code point.
			file: "eq_charCodeAt",
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
			file: "eq_charAt",
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
			file: "eq_indexOf",
			fn:   "find",
			args: [][]any{{"hello", "he"}, {"hello", "lo"}, {"hello", "z"}, {"hello", ""}, {"a😀b", "b"}},
		},
		{
			name: "indexOfFrom",
			// indexOf with the start position: a match before the position is skipped,
			// a match exactly at it counts, a position past the end gives -1 for a
			// non-empty search and the length for the empty search.
			file: "eq_indexOfFrom",
			fn:   "findFrom",
			args: [][]any{{"abcabc", "a", 1}, {"abcabc", "c", 2}, {"abcabc", "a", 4}, {"abc", "", 99}, {"abc", "a", -5}},
		},
		{
			name: "lastIndexOf",
			// lastIndexOf reports the greatest matching index, so it differs from
			// indexOf on a string with two matches; the cases cover the two-match
			// string, a miss, the empty search, and an astral character.
			file: "eq_lastIndexOf",
			fn:   "findLast",
			args: [][]any{{"abcabc", "a"}, {"abcabc", "c"}, {"hello", "z"}, {"hello", ""}, {"a😀b", "b"}},
		},
		{
			name: "includes",
			// The boolean companion of indexOf over the same shapes.
			file: "eq_includes",
			fn:   "has",
			ret:  "boolean",
			args: [][]any{{"hello", "ell"}, {"hello", "z"}, {"hello", ""}, {"a😀b", "😀"}},
		},
		{
			name: "startsWith",
			file: "eq_startsWith",
			fn:   "starts",
			ret:  "boolean",
			args: [][]any{{"hello", "he"}, {"hello", "lo"}, {"hello", ""}, {"hi", "hello"}, {"😀x", "😀"}},
		},
		{
			name: "endsWith",
			// the suffix companion, covering a real suffix, a non-suffix, the empty
			// suffix, a suffix longer than the string, and an astral suffix.
			file: "eq_endsWith",
			fn:   "ends",
			ret:  "boolean",
			args: [][]any{{"hello", "lo"}, {"hello", "he"}, {"hello", ""}, {"hi", "hello"}, {"x😀", "😀"}},
		},
		{
			name: "startsWithPos",
			// startsWith with the position: the prefix must begin at it, so a prefix
			// that matches at 0 fails at a later position and matches at its own.
			file: "eq_startsWithPos",
			fn:   "startsAt",
			ret:  "boolean",
			args: [][]any{{"abcabc", "abc", 3}, {"abcabc", "abc", 1}, {"abcabc", "bc", 1}, {"abc", "", 99}},
		},
		{
			name: "endsWithPos",
			// endsWith with the end position: the window ends there, so a prefix of the
			// string reads as a suffix of the shortened window.
			file: "eq_endsWithPos",
			fn:   "endsAt",
			ret:  "boolean",
			args: [][]any{{"hello", "hell", 4}, {"hello", "lo", 4}, {"hello", "hel", 3}},
		},
		{
			name: "slice2",
			// slice with both bounds over a BMP receiver, covering an interior
			// range, the full string, negative-from-end bounds, an empty result
			// when start is at or past end, and an end past the length.
			file: "eq_slice2",
			fn:   "sl",
			ret:  "string",
			args: [][]any{{"hello", 1, 3}, {"hello", 0, 5}, {"hello", -3, -1}, {"hello", 3, 1}, {"hello", 2, 100}},
		},
		{
			name: "slice1",
			// slice with a single bound proves the optional-argument arity end to
			// end: a positive start, a negative start, and a start past the end.
			file: "eq_slice1",
			fn:   "tail",
			ret:  "string",
			args: [][]any{{"hello", 0}, {"hello", 2}, {"hello", -2}, {"hello", 10}},
		},
		{
			name: "substring",
			// substring differs from slice at the edges: negatives become 0 and a
			// start past end swaps, so the cases pin both behaviors against the
			// engine.
			file: "eq_substring",
			fn:   "sub",
			ret:  "string",
			args: [][]any{{"hello", 1, 3}, {"hello", 3, 1}, {"hello", -2, 3}, {"hello", 2, 100}},
		},
		{
			name: "substr",
			// substr takes a start and a length, unlike slice and substring: a
			// negative start counts from the end, a length past the end clamps, and a
			// zero or negative length is empty, all of which must match the engine.
			file: "eq_string_substr",
			fn:   "sub",
			ret:  "string",
			args: [][]any{{"hello", 1, 3}, {"hello", -2, 5}, {"hello", 2, 100}, {"hello", 1, 0}, {"hello", 10, 2}},
		},
		{
			name: "padStart2",
			// padStart with an explicit pad string: a target longer than the
			// string pads and repeats-then-truncates the pad, a target not longer
			// leaves the string alone, and an empty pad string is a no-op.
			file: "eq_padStart2",
			fn:   "pad",
			ret:  "string",
			args: [][]any{{"5", 3, "0"}, {"5", 1, "0"}, {"5", 6, "ab"}, {"abc", 5, ""}, {"abc", -2, "0"}},
		},
		{
			name: "padStart1",
			// padStart with no pad argument defaults to a space, the optional-arg
			// path over a mixed number-then-string signature.
			file: "eq_padStart1",
			fn:   "padsp",
			ret:  "string",
			args: [][]any{{"7", 4}, {"7", 1}, {"hi", 5}},
		},
		{
			name: "padEnd2",
			// padEnd appends the filler instead of prepending it; same rules.
			file: "eq_padEnd2",
			fn:   "padr",
			ret:  "string",
			args: [][]any{{"5", 3, "0"}, {"5", 6, "ab"}, {"abc", 2, "0"}, {"abc", 5, ""}},
		},
		{
			name: "concatMethod",
			// s.concat(a, b) joins three strings in order; the cases cover an empty
			// receiver, empty arguments, and a multi-byte character so the code-unit
			// join is exercised, all of which must match the engine's concat.
			file: "eq_concatMethod",
			fn:   "j",
			ret:  "string",
			args: [][]any{{"a", "b", "c"}, {"", "x", ""}, {"π", "😀", "z"}},
		},
		{
			name: "mathFloor",
			// Math.floor over positive, negative, and already-integer inputs, where
			// floor differs from truncation on the negative fraction.
			file: "eq_mathFloor",
			fn:   "fl",
			args: [][]any{{3.7}, {-3.2}, {5}, {-0.5}},
		},
		{
			name: "mathCeilTruncAbs",
			// three one-argument methods composed, so the whole chain is checked at
			// once: ceil then abs of a trunc, covering the sign handling each does.
			file: "eq_mathCeilTruncAbs",
			fn:   "c",
			args: [][]any{{3.9}, {-3.9}, {0}},
		},
		{
			name: "mathSqrt",
			// perfect squares so the result is exact on both sides, since IEEE sqrt
			// is correctly rounded and deterministic.
			file: "eq_mathSqrt",
			fn:   "r",
			args: [][]any{{4}, {9}, {2}, {0}},
		},
		{
			name: "mathPow",
			// integer and power-of-two exponents, whose results are exact so the two
			// pow implementations cannot diverge in the last bit.
			file: "eq_mathPow",
			fn:   "p",
			args: [][]any{{2, 10}, {5, 3}, {2, -2}, {9, 0}},
		},
		{
			name: "mathMinMax",
			// min and max fold two numbers; the cases cover the ordinary order, the
			// reversed order, and equal values.
			file: "eq_mathMinMax",
			fn:   "mm",
			args: [][]any{{3, 7}, {7, 3}, {4, 4}, {-1, -5}},
		},
		{
			name: "mathMin3",
			// three arguments through the variadic Math.min, which the two-argument
			// lowering could not do; the cases put the smallest first, middle, and
			// last so the fold order does not matter.
			file: "eq_mathMin3",
			fn:   "m3",
			args: [][]any{{1, 2, 3}, {2, 1, 3}, {3, 2, 1}, {5, 5, 5}},
		},
		{
			name: "mathRound",
			// the half-way inputs are where JavaScript and Go's math.Round disagree:
			// 2.5 rounds to 3 both ways, but -2.5 rounds to -2 in JavaScript (toward
			// +Infinity) and -3 in Go, so this case fails unless value.Round is used.
			file: "eq_mathRound",
			fn:   "rnd",
			args: [][]any{{2.5}, {-2.5}, {2.4}, {-2.6}, {0}},
		},
		{
			name: "mathSign",
			// sign over positive, negative, and zero; the zero passes through so both
			// sides print the same, and the nonzero cases give the constant one.
			file: "eq_mathSign",
			fn:   "sgn",
			args: [][]any{{5}, {-5}, {0}, {0.001}, {-0.001}},
		},
		{
			name: "mathFround",
			// fround snaps to the nearest float32: an exact value is unchanged, 1.1 lands
			// on its float32 neighbor, 2^24+1 rounds down to 2^24 (the first integer
			// float32 cannot hold), and a magnitude past the float32 range goes to
			// infinity, all of which the engine's Math.fround does too.
			file: "eq_mathFround",
			fn:   "f",
			args: [][]any{{1}, {1.1}, {16777217}, {1e39}, {-0.5}},
		},
		{
			name: "mathClz32",
			// clz32 counts leading zeros of the ToUint32 coercion: 0 counts all 32, a
			// power of two counts its position, -1 coerces to all ones so counts 0, and a
			// fraction truncates before the count.
			file: "eq_mathClz32",
			fn:   "c",
			args: [][]any{{0}, {1}, {2}, {-1}, {3.9}},
		},
		{
			name: "mathImul",
			// imul multiplies as 32-bit signed integers, so the small case is ordinary but
			// the large ones overflow and keep only the low 32 bits, wrapping negative the
			// way the engine's Math.imul does; a fraction truncates first.
			file: "eq_mathImul",
			fn:   "m",
			args: [][]any{{3, 4}, {-1, 8}, {4294967295, 5}, {2147483647, 2}, {6.9, 3}},
		},
		{
			name: "bitAndOrXor",
			// the three logical bitwise operators over positive, negative (which
			// exercises the two's-complement ToInt32 wrap), and zero operands.
			file: "eq_bitAndOrXor",
			fn:   "b",
			args: [][]any{{12, 10}, {-1, 255}, {0, 0}, {-8, 3}},
		},
		{
			name: "bitShifts",
			// left and arithmetic-right shift, including a negative left operand so
			// the sign propagation of >> is checked, and a count above 31 to pin the
			// five-bit mask.
			file: "eq_bitShifts",
			fn:   "sh",
			args: [][]any{{1, 4}, {-1, 2}, {255, 1}, {1, 33}},
		},
		{
			name: "bitUnsignedShift",
			// >>> is the operator that most visibly differs from a signed shift:
			// -1 >>> 0 is 4294967295, not -1, so the ToUint32 coercion is what makes
			// the emitted Go agree with the engine.
			file: "eq_bitUnsignedShift",
			fn:   "us",
			args: [][]any{{-1, 0}, {-1, 1}, {256, 2}, {8, 1}},
		},
		{
			name: "bitNot",
			// ~x is -(x+1) on the coerced integer, so the cases cover positive,
			// negative, zero, and a fraction that must truncate first.
			file: "eq_bitNot",
			fn:   "inv",
			args: [][]any{{0}, {5}, {-1}, {6.9}, {-6.9}},
		},
		{
			name: "bitCoerceFraction",
			// a fractional operand must truncate before the bitwise op, the ToInt32
			// step, so 6.9 & 3 is 6 & 3, not a float operation.
			file: "eq_bitCoerceFraction",
			fn:   "f",
			args: [][]any{{6.9, 3}, {-6.9, 3}},
		},
		{
			name: "numberIsInteger",
			// the argument is x / y so the cases can reach a fraction (7/2), a whole
			// number (6/2), and a non-finite value (1/0 is Infinity), all of which
			// Number.isInteger must judge the same way the engine does.
			file: "eq_numberIsInteger",
			fn:   "ii",
			ret:  "boolean",
			args: [][]any{{6, 2}, {7, 2}, {1, 0}, {-4, 2}},
		},
		{
			name: "numberIsFiniteNaN",
			// isFinite and isNaN over the same x / y, covering a finite result, an
			// infinity (1/0), and a NaN (0/0).
			file: "eq_numberIsFiniteNaN",
			fn:   "fn",
			ret:  "boolean",
			args: [][]any{{3, 2}, {1, 0}, {0, 0}},
		},
		{
			name: "globalIsNaNIsFinite",
			// the bare global isNaN and isFinite over x / y, covering a finite result,
			// an infinity (1/0), and a NaN (0/0). On a number argument these coerce to
			// nothing, so they must agree with the engine's coercing globals.
			file: "eq_globalIsNaNIsFinite",
			fn:   "g",
			ret:  "boolean",
			args: [][]any{{3, 2}, {1, 0}, {0, 0}},
		},
		{
			name: "numberIsSafeInteger",
			// x * y lets a case exceed the safe-integer range: 9007199254740992 is
			// 2^53, an integer that is not safe, so the harness pins the boundary.
			file: "eq_numberIsSafeInteger",
			fn:   "si",
			ret:  "boolean",
			args: [][]any{{2, 3}, {4503599627370496, 2}, {4503599627370497, 2}},
		},
		{
			name: "toUpperCase",
			// the inputs are the cases where the full mapping diverges from Go's simple
			// ToUpper: the sharp s expands to SS, a word carries the expansion, a
			// ligature expands, and the emoji has no case, all of which must match the
			// engine's toUpperCase.
			file: "eq_toUpperCase",
			fn:   "up",
			ret:  "string",
			args: [][]any{{"hello"}, {"ß"}, {"straße"}, {"ﬀ"}, {"aπ😀"}},
		},
		{
			name: "toLowerCase",
			// the trailing-sigma inputs pin the Final_Sigma context, where a word-final
			// capital sigma lowercases to ς and a non-final one to σ, which Go's simple
			// ToLower does not do; the dotted capital I expands to i plus a combining
			// dot.
			file: "eq_toLowerCase",
			fn:   "down",
			ret:  "string",
			args: [][]any{{"HELLO"}, {"ΟΔΟΣ"}, {"ΣΣ"}, {"İ"}},
		},
		{
			name: "trim",
			// The inputs carry the exact ECMAScript whitespace set, not just ASCII
			// spaces: a tab and newlines, a no-break space (U+00A0), and a
			// zero-width no-break space (U+FEFF), all of which trim removes and
			// Go's unicode.IsSpace would get wrong. An all-whitespace string trims
			// to empty and a clean string is unchanged.
			file: "eq_trim",
			fn:   "clean",
			ret:  "string",
			args: [][]any{{"  hello  "}, {"\t\n hi \r\n"}, {"\u00a0x\u00a0"}, {"\ufeffy\ufeff"}, {"none"}, {"   "}},
		},
		{
			name: "trimStart",
			// trimStart removes only the leading run, so trailing whitespace must
			// survive, which the engine and the emitted Go must agree on.
			file: "eq_trimStart",
			fn:   "lead",
			ret:  "string",
			args: [][]any{{"  hi  "}, {" x"}, {"none"}},
		},
		{
			name: "trimEnd",
			file: "eq_trimEnd",
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
			file: "eq_strlitEscapes",
			fn:   "lit",
			ret:  "string",
			args: [][]any{{}},
		},
		{
			name: "stringOfNumber",
			// String(x) on a number is the exact ECMAScript Number::toString. The
			// inputs are divided so the case exercises the exponential thresholds and
			// the unpadded exponent that separate it from strconv's 'g': a large
			// integer, the 1e21 boundary, a small decimal, and 1e-7 where JavaScript
			// goes exponential. A fraction and a negative pin the ordinary path.
			file: "eq_stringOfNumber",
			fn:   "show",
			ret:  "string",
			args: [][]any{{1, 2}, {-3, 2}, {6, 2}, {1e21, 1}, {1, 1000000}, {1, 10000000}, {1, 0}, {0, 0}},
		},
		{
			name: "stringOfBool",
			// String(b) on a boolean is "true" or "false"; x < y makes both reachable.
			file: "eq_stringOfBool",
			fn:   "show",
			ret:  "string",
			args: [][]any{{1, 2}, {2, 1}},
		},
		{
			name: "parseIntString",
			// parseInt(s) with no radix: base 10 with 0x detection. The inputs cover a
			// trailing tail, a signed value with whitespace, the 0x prefix, a leading
			// zero (which is not legacy octal), a fraction stopped at the point, and the
			// non-numeric NaN case, all of which must agree with value.ParseInt.
			file: "eq_parseIntString",
			fn:   "pi",
			args: [][]any{{"42px"}, {"  -17  "}, {"0x1F"}, {"010"}, {"3.9"}, {"abc"}, {""}},
		},
		{
			name: "parseIntRadix",
			// parseInt(s, r) with an explicit radix: binary, octal, hex (with and without
			// the prefix), base 36, and the out-of-range radix that is NaN.
			file: "eq_parseIntRadix",
			fn:   "pi",
			args: [][]any{{"101", 2}, {"777", 8}, {"ff", 16}, {"0x1F", 16}, {"0x1F", 10}, {"zz", 36}, {"10", 1}},
		},
		{
			name: "parseFloatString",
			// parseFloat(s) reads the longest decimal prefix and ignores the rest. The
			// inputs cover a bare number, a trailing tail, a leading-whitespace trim, the
			// Infinity word, a dangling exponent marker (where the mantissa alone is the
			// prefix), the radix form parseFloat does not read (so "0x1F" is 0), and a
			// non-numeric string (NaN), all of which must agree with value.ParseFloat.
			file: "eq_parseFloatString",
			fn:   "pf",
			args: [][]any{{"3.14"}, {"42px"}, {"  -2.5  "}, {"1e3rest"}, {"Infinity!"}, {"1e"}, {"0x1F"}, {".5"}, {"abc"}, {""}},
		},
		{
			name: "numberParseInt",
			// Number.parseInt is the same function as the global parseInt, so it must give
			// the same answers over the radix spread: binary, octal, hex with the prefix,
			// base 10 not stripping 0x, base 36, and the out-of-range radix that is NaN.
			file: "eq_numberParseInt",
			fn:   "pi",
			args: [][]any{{"101", 2}, {"777", 8}, {"0x1F", 16}, {"0x1F", 10}, {"zz", 36}, {"10", 1}},
		},
		{
			name: "numberParseFloat",
			// Number.parseFloat is the same function as the global parseFloat, so it reads
			// the same longest decimal prefix over a bare number, a trailing tail, the
			// Infinity word, the radix form it does not read, and a non-numeric string.
			file: "eq_numberParseFloat",
			fn:   "pf",
			args: [][]any{{"3.14"}, {"42px"}, {"Infinity!"}, {"0x1F"}, {"abc"}},
		},
		// The Math and Number constants each take no argument and must return the exact
		// same double the engine reads from the namespace property. A one-bit error in
		// the checked-in decimal shows up here as a diverging result, so these cases are
		// the real proof the constants are bit-exact, including the two infinities and
		// NaN, which the harness canonicalizes to +Inf, -Inf, and NaN on both sides.
		{name: "mathE", file: "eq_math_e", fn: "f", args: [][]any{{}}},
		{name: "mathLN10", file: "eq_math_ln10", fn: "f", args: [][]any{{}}},
		{name: "mathLN2", file: "eq_math_ln2", fn: "f", args: [][]any{{}}},
		{name: "mathLOG10E", file: "eq_math_log10e", fn: "f", args: [][]any{{}}},
		{name: "mathLOG2E", file: "eq_math_log2e", fn: "f", args: [][]any{{}}},
		{name: "mathPI", file: "eq_math_pi", fn: "f", args: [][]any{{}}},
		{name: "mathSQRT1_2", file: "eq_math_sqrt1_2", fn: "f", args: [][]any{{}}},
		{name: "mathSQRT2", file: "eq_math_sqrt2", fn: "f", args: [][]any{{}}},
		{name: "numberEpsilon", file: "eq_number_epsilon", fn: "f", args: [][]any{{}}},
		{name: "numberMaxSafeInteger", file: "eq_number_max_safe_integer", fn: "f", args: [][]any{{}}},
		{name: "numberMinSafeInteger", file: "eq_number_min_safe_integer", fn: "f", args: [][]any{{}}},
		{name: "numberMaxValue", file: "eq_number_max_value", fn: "f", args: [][]any{{}}},
		{name: "numberMinValue", file: "eq_number_min_value", fn: "f", args: [][]any{{}}},
		{name: "numberPositiveInfinity", file: "eq_number_positive_infinity", fn: "f", args: [][]any{{}}},
		{name: "numberNegativeInfinity", file: "eq_number_negative_infinity", fn: "f", args: [][]any{{}}},
		{name: "numberNaN", file: "eq_number_nan", fn: "f", args: [][]any{{}}},
		// Template literals build a string, so each case declares a string return and
		// compares the joined result against the engine's. The head, the coerced
		// substitutions, and the trailing literals must land in the same order with the
		// same ToString, and the escapes must cook identically, or the strings diverge.
		{name: "templateNosub", file: "eq_template_nosub", fn: "f", ret: "string", args: [][]any{{}}},
		{
			name: "templateBasic",
			file: "eq_template_basic",
			fn:   "f",
			ret:  "string",
			// a number and a string interpolated together, over an integer, a negative,
			// and a fraction so the NumberToString of the first substitution is exercised
			// alongside the passed-through string.
			args: [][]any{{0, "x"}, {42, "mid"}, {-7, ""}, {3.5, "π"}},
		},
		{
			name: "templateNumber",
			file: "eq_template_number",
			fn:   "f",
			ret:  "string",
			// a lone number substitution across the Number::toString spread: an integer,
			// a fraction, a negative, zero, and the exponential thresholds, all of which
			// must format the same as the engine inside the template.
			args: [][]any{{0}, {42}, {-7}, {3.5}, {1e21}, {1e-7}, {1e20}},
		},
		{
			name: "templateBool",
			file: "eq_template_bool",
			fn:   "f",
			ret:  "string",
			args: [][]any{{true}, {false}},
		},
		{
			name: "templateEscape",
			file: "eq_template_escape",
			fn:   "f",
			ret:  "string",
			// the cooked head and tail carry a tab, an escaped backtick, and an escaped
			// dollar-brace, so the engine and the generated Go must resolve the same
			// escapes around the substitution.
			args: [][]any{{1}, {-2}},
		},
		{
			name: "stringFromCharCode",
			file: "eq_string_from_char_code",
			fn:   "chars",
			ret:  "string",
			// each argument goes through ToUint16, so the cases cover plain ASCII, a
			// value past 2^16 that must wrap, a fraction that must truncate, and a
			// surrogate pair that rejoins into one astral rune. The engine and
			// value.FromCharCode must agree code unit for code unit.
			args: [][]any{{72, 105, 33}, {65536 + 66, 67.9, 68}, {0xD83D, 0xDE00, 65}},
		},
		{
			name: "stringReplace",
			file: "eq_string_replace",
			fn:   "rep",
			ret:  "string",
			// only the first match is replaced, a missing search returns the receiver,
			// an empty search inserts at the front, and the substitution patterns $&,
			// $` and $' and the literal $$ must expand the same as the engine.
			args: [][]any{
				{"abcabc", "bc", "X"},
				{"hello", "xyz", "Q"},
				{"abc", "", "-"},
				{"a.b", ".", "[$&]"},
				{"one two", "two", "<$`>"},
				{"cost", "cost", "$$5"},
			},
		},
		{
			name: "stringReplaceAll",
			file: "eq_string_replace_all",
			fn:   "repAll",
			ret:  "string",
			// every match is replaced, the replacement is not rescanned, an empty
			// search weaves at every gap, and $& expands per match.
			args: [][]any{
				{"abcabc", "bc", "X"},
				{"aaa", "a", "aa"},
				{"abc", "", "-"},
				{"a.b.c", ".", "[$&]"},
			},
		},
		{
			name: "numberToString",
			file: "eq_number_tostring",
			fn:   "show",
			ret:  "string",
			// toString spans the Number::toString cases: an integer, a fraction, a
			// negative, the exponential thresholds, and the non-finite values, all of
			// which must format the same as the engine.
			args: [][]any{{0}, {42}, {-7}, {3.5}, {1e21}, {1e-7}, {1e20}},
		},
		{
			name: "numberValueOf",
			file: "eq_number_valueof",
			fn:   "id",
			ret:  "", // number
			// valueOf returns the primitive itself, so the generated Go and the engine
			// must return the identical number across the same spread.
			args: [][]any{{0}, {42}, {-7}, {3.5}, {1e21}},
		},
		{
			name: "boolToString",
			file: "eq_bool_tostring",
			fn:   "show",
			ret:  "string",
			// toString on a boolean is the word, not a number, for both values.
			args: [][]any{{true}, {false}},
		},
		{
			name: "booleanOfNumber",
			// Boolean(x) on a number is false only at zero or NaN. The division reaches
			// a nonzero result, +0, and NaN (0/0), the last being what a bare zero test
			// would call truthy, so the engine and value.NumberToBool must agree.
			file: "eq_booleanOfNumber",
			fn:   "ok",
			ret:  "boolean",
			args: [][]any{{6, 2}, {0, 1}, {0, 0}, {-3, 2}},
		},
		{
			name: "booleanOfString",
			// Boolean(s) on a string is false only when empty; content does not matter,
			// so "0" and "false" are truthy and must agree with value.StringToBool.
			file: "eq_booleanOfString",
			fn:   "ok",
			ret:  "boolean",
			args: [][]any{{""}, {"a"}, {" "}, {"0"}, {"false"}, {"😀"}},
		},
		{
			name: "numberOfString",
			// Number(s) on a string is the exact ECMAScript ToNumber over the
			// StrNumericLiteral grammar. The inputs cover a trimmed decimal, a bare
			// fraction, the radix prefixes, the Infinity word, the empty string (which
			// is 0), and a non-numeric string (which is NaN), all of which the engine
			// and value.StringToNumber must agree on, including the NaN result.
			file: "eq_numberOfString",
			fn:   "num",
			args: [][]any{{"  42  "}, {".5"}, {"1e3"}, {"0x1F"}, {"0b101"}, {"0o17"}, {"Infinity"}, {"-Infinity"}, {""}, {"abc"}, {"1_000"}},
		},
		{
			name: "numberOfBool",
			// Number(b) on a boolean is 1 or 0; x < y makes both reachable.
			file: "eq_numberOfBool",
			fn:   "num",
			args: [][]any{{1, 2}, {2, 1}},
		},
		{
			name: "numLiterals",
			// the numeric-literal forms this slice lowers: hex, binary, and octal
			// integers, an underscore-separated decimal, and an exponent. The compiler
			// emits each as the Go literal for the same value and the engine parses the
			// same source, so their sum must agree. A zero-argument function.
			file: "eq_numLiterals",
			fn:   "n",
			args: [][]any{{}},
		},
		{
			name: "modulo",
			file: "eq_modulo",
			// fmod keeps the sign of the dividend and works on fractions, so the
			// cases cover negative dividends and a non-integer operand, where a
			// naive integer remainder would diverge from JavaScript.
			fn:   "rem",
			args: [][]any{{7, 3}, {-7, 3}, {7, -3}, {5.5, 2}, {10, 10}},
		},
		{
			name: "logical",
			file: "eq_logical",
			fn:   "between",
			args: [][]any{{5, 0, 10}, {-1, 0, 10}, {11, 0, 10}, {0, 0, 10}, {10, 0, 10}},
		},
		{
			name: "crossCall",
			file: "eq_crossCall",
			fn:   "hypotSq",
			args: [][]any{{3, 4}, {0, 0}, {-2, 5}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := readTS(t, tc.file)
			goSrc, imports := lowerToGo(t, src)
			goName, ok := exportedField(tc.fn)
			if !ok {
				t.Fatalf("entry %q is not a Go identifier", tc.fn)
			}
			for _, args := range tc.args {
				want := evalTS(t, src, tc.fn, tc.ret, args)
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
