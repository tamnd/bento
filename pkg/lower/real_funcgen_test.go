package lower

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// readTS reads the TypeScript source for a golden case from its checked-in .ts
// input under testdata, the "original TypeScript" half of each golden pair.
func readTS(t *testing.T, stem string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("testdata", stem+".ts"))
	if err != nil {
		t.Fatalf("read TS input %s.ts: %v", stem, err)
	}
	return string(src)
}

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
		name, file string
	}{
		{name: "identity", file: "func_identity"},
		{name: "add", file: "func_add"},
		// exercises every mapped operator plus parentheses and a numeric literal, all on float64.
		{name: "arithmetic", file: "func_arithmetic"},
		// a parameter named for a Go keyword must mangle to type_ on both its declaration and its use, and stay consistent between them.
		{name: "keyword_param", file: "func_keyword_param"},
		// exercises local var declarations, a while loop, an if/else, a relational and a strict-equality condition, and assignment, so a whole control-flow body is pinned as one golden.
		{name: "loop", file: "func_loop"},
		// a self-recursive call resolves to the same exported Go name the declaration gets, so the call and its target agree.
		{name: "recursion", file: "func_fib"},
		// string is value.BStr, a string literal is value.FromGoString, and + on two strings is value.Concat, not a Go + which would be UTF-8.
		{name: "concat", file: "func_concat"},
		// a literal whose escapes decode to a valid Go string becomes value.FromGoString of the decoded text, so \n and \t are real control characters and \u{1F600} is the emoji in the emitted Go literal.
		{name: "strlit_escapes", file: "func_strlit_escapes"},
		// a literal whose \u escape names a lone surrogate cannot be a Go string, so it lowers to value.FromUTF16 of the raw code units.
		{name: "strlit_lone_surrogate", file: "func_strlit_lone_surrogate"},
		// .length on a string is the UTF-16 code-unit count, the value.BStr Length method, a float64 that matches the number type .length has.
		{name: "strlen", file: "func_strlen"},
		// === on two strings compares by UTF-16 code unit, the value.BStr Equal method, not a Go == on the struct, which would compare backing fields and misjudge two equal strings backed differently.
		{name: "streq", file: "func_streq"},
		// !== is the negation of the same Equal call, so it lowers to a Go unary not over value.BStr.Equal rather than a struct !=.
		{name: "strneq", file: "func_strneq"},
		// < on two strings is not a Go operator on the struct: it orders by UTF-16 code unit through value.BStr.Compare, so it lowers to Compare(a, b) < 0.
		{name: "strlt", file: "func_strlt"},
		// >= lowers to the same Compare call against zero with the matching Go token.
		{name: "strge", file: "func_strge"},
		// s.charCodeAt(i) is a method call on a string receiver, so it lowers to the value.BStr CharCodeAt method rather than a plain function call or an index expression.
		{name: "charcode", file: "func_charcode"},
		// s.charAt(i) is a string-returning string method, so it lowers to the value.BStr CharAt method and the whole function returns value.BStr.
		{name: "charat", file: "func_charat"},
		// a string method that takes a string argument, so the argument-kind guard admits it where the number-arg methods would reject it.
		{name: "indexof", file: "func_indexof"},
		// indexOf with the optional start position, which lowers to the variadic IndexOf with a second argument the source supplied.
		{name: "indexof_pos", file: "func_indexof_pos"},
		// lastIndexOf takes the same string-then-optional-number shape as indexOf.
		{name: "lastindexof", file: "func_lastindexof"},
		// a string-arg method returning a boolean, so the whole function is bool-typed.
		{name: "includes", file: "func_includes"},
		{name: "startswith", file: "func_startswith"},
		// the suffix companion of startsWith, the same string-then-optional-number shape.
		{name: "endswith", file: "func_endswith"},
		// startsWith with the optional position, which lowers to the variadic StartsWith with the second argument the source supplied.
		{name: "startswith_pos", file: "func_startswith_pos"},
		// slice with both arguments; the Go method is variadic but the call passes exactly the two the source wrote.
		{name: "slice2", file: "func_slice2"},
		// slice with one argument exercises the optional-argument arity: the same variadic method, called with a single index.
		{name: "slice1", file: "func_slice1"},
		{name: "substring", file: "func_substring"},
		// padStart with both a number and a string argument exercises the mixed argument-kind guard: arg 0 must be a number and arg 1 a string, where the number-only methods would reject the string.
		{name: "padstart2", file: "func_padstart2"},
		// padStart with only the length argument exercises the optional pad string: the same variadic Go method, called without the pad, which then defaults to a space.
		{name: "padstart1", file: "func_padstart1"},
		{name: "padend2", file: "func_padend2"},
		// s.concat(a, b) is the variadic concat method, not the + operator, so it lowers to the value.BStr ConcatN method with every argument passed through; the variadic arity admits the two string arguments.
		{name: "concat_method", file: "func_concat_method"},
		// Math.floor is a call on the global Math namespace, so the receiver is not lowered to a value; it becomes the math package qualifier and the call is math.Floor.
		{name: "math_floor", file: "func_math_floor"},
		// a two-argument Math method lowers to a two-argument math function.
		{name: "math_pow", file: "func_math_pow"},
		// Math.max takes any number of arguments, so it lowers to the variadic value.MaxN rather than the two-argument math.Max.
		{name: "math_max", file: "func_math_max"},
		// three arguments, which the old two-argument arity would have handed back; value.MinN takes the whole list.
		{name: "math_min3", file: "func_math_min3"},
		// Math calls compose: the argument to one is the result of another, so the whole expression lowers to nested math calls.
		{name: "math_nested", file: "func_math_nested"},
		// Math.round does not lower to math.Round: JavaScript breaks a tie toward +Infinity, so the call goes to value.Round which carries that rule.
		{name: "math_round", file: "func_math_round"},
		// Go has no math.Sign, so Math.sign lowers to value.Sign.
		{name: "math_sign", file: "func_math_sign"},
		// Math.fround is a single-precision round trip, bit-exact, so it lowers to value.Fround rather than any math package function.
		{name: "math_fround", file: "func_math_fround"},
		// Math.clz32 counts leading zeros of the ToUint32 coercion, so it lowers to value.Clz32.
		{name: "math_clz32", file: "func_math_clz32"},
		// Math.imul is a two-argument 32-bit integer multiply, so it lowers to value.Imul with both arguments passed through.
		{name: "math_imul", file: "func_math_imul"},
		// the transcendental Math methods map straight onto the math package: sin, log, and the two-argument atan2.
		{name: "math_transcendental", file: "func_math_transcendental"},
		// String(x) on a number is the ECMAScript Number::toString, which lowers to value.NumberToString rather than strconv, whose exponent thresholds and padding differ.
		{name: "string_of_number", file: "func_string_of_number"},
		// String(b) on a boolean lowers to value.BoolToString.
		{name: "string_of_bool", file: "func_string_of_bool"},
		// String(s) on a string is the identity, so it lowers to the argument unchanged with no call wrapped around it.
		{name: "string_of_string", file: "func_string_of_string"},
		// Number(s) on a string is the ECMAScript ToNumber over the StrNumericLiteral grammar, which lowers to value.StringToNumber rather than strconv, whose grammar accepts forms JavaScript rejects.
		{name: "number_of_string", file: "func_number_of_string"},
		// Number(b) on a boolean lowers to value.BoolToNumber, true to 1 and false to 0.
		{name: "number_of_bool", file: "func_number_of_bool"},
		// Number(n) on a number is the identity, so it lowers to the argument unchanged with no call wrapped around it.
		{name: "number_of_number", file: "func_number_of_number"},
		// Boolean(x) on a number lowers to value.NumberToBool, false only at zero or NaN.
		{name: "boolean_of_number", file: "func_boolean_of_number"},
		// Boolean(s) on a string lowers to value.StringToBool, false only when empty.
		{name: "boolean_of_string", file: "func_boolean_of_string"},
		// Boolean(b) on a boolean is the identity, so it lowers to the argument unchanged with no call wrapped around it.
		{name: "boolean_of_bool", file: "func_boolean_of_bool"},
		// parseFloat(s) reads the longest decimal prefix of a string, which lowers to value.ParseFloat, the lenient parse distinct from Number().
		{name: "parse_float", file: "func_parse_float"},
		// parseInt(s) with no radix lowers to value.ParseInt with a literal 0, which the value function treats as the omitted-argument case (base 10 with 0x detection).
		{name: "parse_int", file: "func_parse_int"},
		// parseInt(s, r) passes the radix through to value.ParseInt.
		{name: "parse_int_radix", file: "func_parse_int_radix"},
		// a hexadecimal integer literal is a number like any other, so it lowers to the same hex spelling as a Go int constant, added as a float64.
		{name: "num_hex", file: "func_num_hex"},
		// underscore digit separators are stripped, so the emitted Go literal is the same value with no separators.
		{name: "num_separators", file: "func_num_separators"},
		// an exponent literal carries the .eE that marks it a Go float constant, so it stays a float literal in the emitted code.
		{name: "num_exponent", file: "func_num_exponent"},
		// & on numbers is not a Go & on float64; each operand coerces through value.ToInt32, the operator runs on the ints, and the result casts back to float64.
		{name: "bit_and", file: "func_bit_and"},
		// | and ^ compose the same way, so a nested expression pins both in one golden.
		{name: "bit_or_xor", file: "func_bit_or_xor"},
		// << masks the shift count to five bits through value.ToUint32, so the right operand lowers differently from the left.
		{name: "bit_shl", file: "func_bit_shl"},
		// >> is an arithmetic shift, carried by the signed ToInt32 left operand.
		{name: "bit_shr", file: "func_bit_shr"},
		// >>> is a logical shift: the left operand coerces with ToUint32 so Go's >> on an unsigned type is logical and the result is non-negative.
		{name: "bit_ushr", file: "func_bit_ushr"},
		// Number.isInteger is a static call on the global Number namespace, so it lowers to the value.NumberIsInteger predicate and the function returns bool.
		{name: "number_isinteger", file: "func_number_isinteger"},
		{name: "number_isnan", file: "func_number_isnan"},
		// Number.parseInt is the same function as the global parseInt, so it routes to the same lowering: value.ParseInt with the radix passed through.
		{name: "number_parseint", file: "func_number_parseint"},
		// Number.parseFloat is the same function as the global parseFloat, so it lowers to value.ParseFloat.
		{name: "number_parsefloat", file: "func_number_parsefloat"},
		// the Math numeric constants are property reads on the global namespace, not method calls, so they lower through propertyAccess to the exact value-package constant.
		{name: "math_e", file: "func_math_e"},
		{name: "math_ln10", file: "func_math_ln10"},
		{name: "math_ln2", file: "func_math_ln2"},
		{name: "math_log10e", file: "func_math_log10e"},
		{name: "math_log2e", file: "func_math_log2e"},
		{name: "math_pi", file: "func_math_pi"},
		{name: "math_sqrt1_2", file: "func_math_sqrt1_2"},
		{name: "math_sqrt2", file: "func_math_sqrt2"},
		// the Number constants lower the same way; the finite ones are value constants, the three non-finite ones (the infinities and NaN) are calls that build the value.
		{name: "number_epsilon", file: "func_number_epsilon"},
		{name: "number_max_safe_integer", file: "func_number_max_safe_integer"},
		{name: "number_min_safe_integer", file: "func_number_min_safe_integer"},
		{name: "number_max_value", file: "func_number_max_value"},
		{name: "number_min_value", file: "func_number_min_value"},
		{name: "number_positive_infinity", file: "func_number_positive_infinity"},
		{name: "number_negative_infinity", file: "func_number_negative_infinity"},
		{name: "number_nan", file: "func_number_nan"},
		// a no-substitution template literal is exactly a string of its cooked content, so it lowers to the same value.FromGoString a string literal would.
		{name: "template_nosub", file: "func_template_nosub"},
		// a template with substitutions joins the head, each coerced expression, and the following literal with one ConcatN; the number goes through NumberToString and the string passes through.
		{name: "template_basic", file: "func_template_basic"},
		// a boolean substitution coerces through value.BoolToString, the same ToString String(b) uses.
		{name: "template_bool", file: "func_template_bool"},
		// the cooked parts resolve escapes the same as a string literal, including a tab, an escaped backtick, and an escaped dollar-brace that is a literal rather than a substitution.
		{name: "template_escape", file: "func_template_escape"},
		// String.fromCharCode is a static call on the global String constructor, so it lowers to the variadic value.FromCharCode with each number argument passed through.
		{name: "string_fromcharcode", file: "func_string_fromcharcode"},
		// with no arguments fromCharCode is still value.FromCharCode, an empty call that yields the empty string.
		{name: "string_fromcharcode_empty", file: "func_string_fromcharcode_empty"},
		// the string-string form of replace lowers to value.BStr.Replace; a regexp or function argument would not type as a string and would hand back.
		{name: "string_replace", file: "func_string_replace"},
		// replaceAll lowers to value.BStr.ReplaceAll the same way.
		{name: "string_replace_all", file: "func_string_replace_all"},
		// toString on a number receiver is the same coercion String(x) uses, so it lowers to value.NumberToString.
		{name: "number_tostring", file: "func_number_tostring"},
		// valueOf on a number returns the primitive itself, so it lowers to the receiver expression unchanged.
		{name: "number_valueof", file: "func_number_valueof"},
		// toString on a boolean lowers to value.BoolToString, the coercion String(b) uses.
		{name: "bool_tostring", file: "func_bool_tostring"},
		// the legacy substr takes a start and a length, so it lowers to value.BStr.Substr like slice and substring lower to theirs.
		{name: "string_substr", file: "func_string_substr"},
		// the bare global isNaN, not the Number static one, so the callee is a plain identifier. On a number argument it coerces to nothing, so it lowers to the same value.NumberIsNaN predicate.
		{name: "global_isnan", file: "func_global_isnan"},
		// the bare global isFinite lowers to value.NumberIsFinite the same way.
		{name: "global_isfinite", file: "func_global_isfinite"},
		// ~ is the unary bitwise operator, so it uses the same ToInt32 coercion as the binary bitwise operators: float64(^value.ToInt32(x)), not a Go ^ on the float64.
		{name: "bit_not", file: "func_bit_not"},
		// a zero-argument string-returning method that lowers to the value.BStr ToUpperCase method, the full Unicode uppercase mapping rather than Go's simple one.
		{name: "touppercase", file: "func_touppercase"},
		{name: "tolowercase", file: "func_tolowercase"},
		// a zero-argument string method, so the parameter list is empty and the call takes no arguments.
		{name: "trim", file: "func_trim"},
		{name: "trimstart", file: "func_trimstart"},
		{name: "trimend", file: "func_trimend"},
		// % on numbers is fmod, not Go's integer remainder, so it lowers to a math.Mod call rather than a Go % operator.
		{name: "modulo", file: "func_modulo"},
		// && and || on boolean operands map to Go's short-circuit operators, so a compound range check lowers to one Go condition.
		{name: "logical", file: "func_logical"},
		// a C-style for becomes a Go block holding the let declaration and a for with an empty init, so the loop variable keeps its float64 type; the return negates with a unary minus.
		{name: "for_loop", file: "func_for"},
		// a postfix ++ in the for-post clause becomes an idiomatic Go IncDecStmt; the arithmetic compound assignments desugar to x = x <op> rhs so they reuse the binary lowering.
		{name: "compound_arith", file: "func_compound_arith"},
		// += on strings routes through the string-concat path of the shared binary lowering, so it emits value.Concat rather than a Go += that a BStr struct would reject.
		{name: "compound_string", file: "func_compound_string"},
		// %= reuses the remainder path, so it emits math.Mod, not a Go %= that a float64 would reject.
		{name: "compound_mod", file: "func_compound_mod"},
		// ++ and -- on a number local map to Go's IncDecStmt, which accepts a float64.
		{name: "incdec", file: "func_incdec"},
		// a ternary lowers to an immediately-invoked function so only the taken branch runs; a chained ternary nests one inside the other's else return.
		{name: "conditional", file: "func_conditional"},
		// a ternary whose branches are strings types the IIFE result as value.BStr.
		{name: "conditional_string", file: "func_conditional_string"},
		// an array literal lowers to value.NewArray instantiated at the element type, .length on the array to the Len method, and for...of to a Go range over the backing slice.
		{name: "array_forof", file: "func_array_forof"},
		// the element type threads through the whole path: a string[] parameter is *value.Array[value.BStr], for...of binds each element as a value.BStr, and .length on that element is the string length.
		{name: "array_strlen", file: "func_array_strlen"},
		// push is a mutating array method called as a statement: it lowers to the value.Array Push method wrapped in an expression statement, with the variadic form passing several arguments in one call.
		{name: "array_push", file: "func_array_push"},
		// an index expression a[i] lowers to the value.Array At method: both an array variable and an array literal are indexed, and the index is a bitwise expression, so the number index threads through with no conversion.
		{name: "array_index", file: "func_array_index"},
		// + where one operand is a string is concatenation with coercion: a number operand becomes value.NumberToString and a boolean operand value.BoolToString before value.Concat, so a mixed "n=" + n + " even=" + even chain lowers without reaching the number/bool operator dispatch.
		{name: "concat_coerce", file: "func_concat_coerce"},
		// number.toString(radix) with a literal radix lowers to value.NumberToStringRadix folding the radix in, radix 16 and 2 taking that path and a bare toString() routing through the same NumberToString the radix-10 coercion uses.
		{name: "number_radix", file: "func_number_radix"},
		// number.toFixed(digits) with a literal digit count lowers to value.NumberToFixed folding the count in, so a fixed-point format at zero, two, and four fraction digits is emitted over a fractional value.
		{name: "number_fixed", file: "func_number_fixed"},
		// an object literal lowers to a composite literal building a pointer to the interned struct the shape maps to, a shorthand property and a keyed property both becoming keyed fields, and a later o.field read lowers to the matching Go struct field.
		{name: "object_literal", file: "func_object_literal"},
		// map over a concise-body arrow lowers to the value.Array Map method taking a Go function literal, the arrow's parameter typed from the checker and its body returning the element type.
		{name: "array_map", file: "func_array_map"},
		// a map whose callback returns a different type than the element (number to string here) cannot use the Map method, since a Go method may not add a type parameter, so it lowers to the free function value.MapArray[float64, value.BStr] with both type arguments named.
		{name: "array_map_change", file: "func_array_map_change"},
		// filter over a concise-body arrow lowers to the value.Array Filter method, the callback returning a bool with no same-type restriction.
		{name: "array_filter", file: "func_array_filter"},
		// slice lowers to the value.Array Slice method taking its zero, one, or two number bounds variadically, so slice(), slice(start), and slice(start, end) all reach the one method.
		{name: "array_slice", file: "func_array_slice"},
		// indexOf and includes lower to the value.Array search methods, each passing the target and a synthesized element-equality closure: Go == for a number (includes adding the NaN-matches-NaN case), value.BStr.Equal for a string.
		{name: "array_indexof", file: "func_array_indexof"},
		// join lowers to the value.Array Join method, passing the separator (the lowered string argument or the default comma when there is none) and a synthesized per-element ToString closure.
		{name: "array_join", file: "func_array_join"},
		// pop lowers to the value.Array Pop method, which returns value.Opt[T]; the T | undefined result type is the optional shape, and the pop() !== undefined guard lowers to the optional's IsUndefined presence check, negated.
		{name: "opt_pop_drain", file: "func_opt_pop_drain"},
		// a binding of an optional (T | undefined) is a value.Opt[T] variable, so this pins the union-to-Opt type lowering in the const declaration alongside the !== undefined presence test.
		{name: "opt_pop_has", file: "func_opt_pop_has"},
		// inside a !== undefined guard the checker narrows the optional binding to its inner T, so a read of it there unwraps with .Get() (the stored value pulled out of the option), while the guard condition itself still tests the bare Opt.
		{name: "opt_narrow", file: "func_opt_narrow"},
		// s.repeat(n) lowers to the value.BStr Repeat method taking the count as a number, the count coerced and range-checked at runtime by the method the way String.prototype.repeat is.
		{name: "repeat", file: "func_repeat"},
		// s.split(sep) lowers to the value.BStr Split method returning *value.Array[value.BStr], the pieces the string separator cuts, so a chained join has an array to fold.
		{name: "split", file: "func_split"},
		// s.replace(/word/g, r) with a plain-literal global regexp lowers to the value.BStr ReplaceAll method over the literal the pattern spells, the global flag selecting ReplaceAll over Replace.
		{name: "regex_replace", file: "func_regex_replace"},
		// a typed empty-array binding lowers to value.NewArray at the binding's element type (the checker types a bare [] as never[], so the element type comes from the annotation), an unbraced for body lowers as a wrapped block, and push and length lower to the value.Array methods.
		{name: "pushloop", file: "func_pushloop"},
		// JSON.stringify(x) is a static call on the global JSON namespace, so the receiver is not lowered to a value; it becomes value.JSONStringify with the argument boxed as any for the serializer's reflection walk.
		{name: "json_stringify", file: "func_json_stringify"},
		// JSON.parse(s) returns any, so the binding lowers to a boxed value.Value; a property read on that dynamic receiver dispatches through Get, and the number return coerces the boxed length through ToNumber.
		{name: "json_parse", file: "func_json_parse"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, got, err := renderFunc(t, readTS(t, tc.file))
			if err != nil {
				t.Fatalf("RenderFunc(%s): %v", tc.name, err)
			}
			checkGolden(t, tc.file+".golden", got)
		})
	}
}

// TestRenderFuncVoidReturn pins that a function with no return value lowers to a
// Go function with no result list.
func TestRenderFuncVoidReturn(t *testing.T) {
	_, got, err := renderFunc(t, readTS(t, "func_void"))
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
		// for-of over an array lowers now, but over a string (a non-array iterable)
		// is still a later slice, so it hands back.
		{"forOfString", "export function s(x: string): number { let n = 0; for (const ch of x) { n = n + 1; } return n; }"},
		// a prefix increment used as a value needs its pre-increment result, not just
		// the mutation, so it hands back; the statement form (++b;) does lower.
		{"prefixIncrValue", "export function p(a: number): number { let b = a; const c = ++b; return c; }"},
		// a ternary whose branches are different primitives is a union value, which
		// needs the tagged union; a same-primitive ternary does lower.
		{"ternaryMixed", "export function m(c: boolean, n: number, s: string): number { const x = c ? n : s; return n; }"},
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
			t.Parallel()
			_, _, err := renderFunc(t, tc.src)
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderFunc(%s) err = %v, want a *NotYetLowerable", tc.name, err)
			}
		})
	}
}
