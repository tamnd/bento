package lower

import (
	"strings"
	"testing"
)

// TestTupleEmitsStruct pins the type half of tuple lowering: a tuple lowers to an
// interned positional struct with one E<i> field per element, the field types the
// element types lower to, and a slot typed as the tuple takes the struct by value
// rather than by pointer (typed/05 T7, delivery slice 5).
func TestTupleEmitsStruct(t *testing.T) {
	const src = "export function first(pair: [string, number]): number { return pair[1]; }\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "type Tuple_str_num struct {") {
		t.Errorf("tuple did not emit its positional struct:\n%s", got)
	}
	if !strings.Contains(got, "E0 value.BStr") || !strings.Contains(got, "E1 float64") {
		t.Errorf("tuple struct fields are not the positional element types:\n%s", got)
	}
}

// TestTupleElementRead proves a literal-index read t[i] lowers to the struct field
// selector t.E<i>, the read that replaces the array At once the position is fixed
// and typed.
func TestTupleElementRead(t *testing.T) {
	const src = "export function snd(pair: [string, number]): number { return pair[1]; }\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, ".E1") {
		t.Errorf("tuple element read did not lower to a field selector:\n%s", got)
	}
}

// TestTupleLiteral proves a contextually-typed tuple literal builds the struct with
// one keyed field per element, each element value coerced to its field type, and as
// a value composite with no leading pointer.
func TestTupleLiteral(t *testing.T) {
	const src = "export function make(): [string, number] { return [\"age\", 42]; }\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "Tuple_str_num{E0:") || !strings.Contains(got, "E1: 42}") {
		t.Errorf("tuple literal did not build a keyed value composite:\n%s", got)
	}
	if strings.Contains(got, "&Tuple_str_num{") {
		t.Errorf("tuple literal built a pointer composite; a tuple is a value type:\n%s", got)
	}
}

// TestTupleDestructure proves a variable-declaration array destructure of a tuple
// binds each name from its positional field, const [a, b] = pair becoming
// a, b := pair.E0, pair.E1, and that a call source is evaluated once into a
// temporary the field reads select off.
func TestTupleDestructure(t *testing.T) {
	const src = `function pair(): [string, number] { return ["age", 42]; }
export function use(): number {
  const [label, value] = pair();
  return value + label.length;
}
`
	got := renderProgram(t, src)
	if !strings.Contains(got, ".E0") || !strings.Contains(got, ".E1") {
		t.Errorf("tuple destructure did not read positional fields:\n%s", got)
	}
}

// TestTupleReadonlyEmitsSameStruct proves a readonly tuple shares the one struct
// its mutable twin interns: readonly is a checker-only guarantee, so the two are the
// same Go value shape and structurally-equal readonly and mutable tuples stay
// Go-assignable (typed/05 T7).
func TestTupleReadonlyEmitsSameStruct(t *testing.T) {
	const src = `export function f(a: readonly [string, number], b: [string, number]): number {
  return a[1] + b[1];
}
`
	got := renderProgram(t, src)
	if strings.Count(got, "type Tuple_str_num struct {") != 1 {
		t.Errorf("readonly and mutable tuple did not intern to one struct:\n%s", got)
	}
}

// TestTupleOptionalElementInterns pins that a tuple carrying an optional element now
// interns to a struct whose optional position is a value.Opt field, so a required
// read off it (pair[0]) lowers rather than hands back. The value.Opt lowering of the
// optional element replaced this slice's earlier handback.
func TestTupleOptionalElementInterns(t *testing.T) {
	const src = "export function f(pair: [string, number?]): number { return pair[0].length; }\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "E1 value.Opt[float64]") {
		t.Errorf("optional tuple element did not emit a value.Opt field:\n%s", got)
	}
}

// TestTupleHandsBackRestTail pins the zero-fail edge for a rest tail, whose slice
// field is a later sub-slice: a tuple with a trailing rest hands back rather than
// drop the tail.
func TestTupleHandsBackRestTail(t *testing.T) {
	const src = "export function f(pair: [string, ...number[]]): number { return pair[0].length; }\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "rest tail") {
		t.Fatalf("rest-tail tuple handback reason = %q, want a rest-tail reason", reason)
	}
}

// TestTupleHandsBackDynamicIndex pins the zero-fail edge for a non-literal tuple
// index: a read through a variable index has no static field to select, so it hands
// back rather than emit a field the struct may not carry.
func TestTupleHandsBackDynamicIndex(t *testing.T) {
	const src = `export function f(pair: [string, number], i: number): number {
  return pair[i] as number;
}
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "tuple element access") {
		t.Fatalf("dynamic-index tuple handback reason = %q, want a tuple-index reason", reason)
	}
}

// TestTupleAssignDestructureReadsFields proves an assignment-form array destructure of
// a tuple, [a, b] = pair into already-declared locals, lowers to the parallel field
// assignment a, b = pair.E0, pair.E1, the read-into-existing-locals sibling of the
// const [a, b] = pair bind.
func TestTupleAssignDestructureReadsFields(t *testing.T) {
	const src = `export function use(pair: [string, number]): number {
  let label = "";
  let value = 0;
  [label, value] = pair;
  return value + label.length;
}
`
	got := renderProgram(t, src)
	if !strings.Contains(got, ".E0") || !strings.Contains(got, ".E1") {
		t.Errorf("assignment-form tuple destructure did not read positional fields:\n%s", got)
	}
	if strings.Contains(got, ".AtI(") {
		t.Errorf("assignment-form tuple destructure should read fields, not array positions:\n%s", got)
	}
}

// TestTupleAssignDestructureRuns builds and runs the assignment-form destructure so
// the field reads are proven to pick the right positions, including a pattern that
// binds fewer names than the tuple has and a three-element heterogeneous tuple.
func TestTupleAssignDestructureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const pair: [string, number] = ["k", 7];
let a = "";
let b = 0;
[a, b] = pair;
console.log(a + ":" + b);
let first = "";
[first] = pair;
console.log(first);
const trip: [number, string, boolean] = [1, "two", true];
let n = 0;
let s = "";
let flag = false;
[n, s, flag] = trip;
console.log(n + s + flag);
`
	want := "k:7\nk\n1twotrue\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("assignment-form tuple destructure printed %q, want %q", got, want)
	}
}

// TestTupleAssignDestructureHandsBackTypeMismatch pins the zero-fail edge where a
// target's Go type differs from the tuple element it reads: a target widened to a
// union does not render to the element's field type, so a = pair.E0 would not be a
// well-typed Go assignment and the whole statement hands back.
func TestTupleAssignDestructureHandsBackTypeMismatch(t *testing.T) {
	const src = `export function f(pair: [string, number]): void {
  let a: string | number = "";
  let b = 0;
  [a, b] = pair;
}
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "target's type differs from the tuple element type") {
		t.Fatalf("type-mismatch handback reason = %q, want a target-type-differs reason", reason)
	}
}

// TestTupleOptionalElementStruct pins the struct shape for a tuple carrying an
// optional element: the optional position emits a value.Opt[T] field, the required
// position stays its bare Go type, and the whole tuple no longer hands back.
func TestTupleOptionalElementStruct(t *testing.T) {
	const src = `const a: [number, string?] = [1, "x"];
console.log(a[0] + "");
`
	got := renderProgram(t, src)
	if !strings.Contains(got, "E1 value.Opt[value.BStr]") {
		t.Errorf("optional tuple element did not emit a value.Opt field:\n%s", got)
	}
	if !strings.Contains(got, "E0 float64") {
		t.Errorf("required tuple element should stay its bare type:\n%s", got)
	}
}

// TestTupleOptionalElementRuns builds and runs a tuple with an optional element,
// covering a present value (someWrap), an omitted trailing optional (noneOf), a
// presence test t[i] !== undefined that reads the Opt, and a narrowed read past the
// guard that unwraps with .Get().
func TestTupleOptionalElementRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const a: [number, string?] = [1, "x"];
const b: [number, string?] = [2];
console.log(a[0] + "");
console.log(a[1] !== undefined ? a[1] : "none");
console.log(b[1] !== undefined ? b[1] : "none");
`
	want := "1\nx\nnone\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("optional tuple element printed %q, want %q", got, want)
	}
}

// TestTupleOptionalShapeMismatchHandsBack pins the zero-fail edge: a literal built at
// its own required arity flowing into a slot whose declared tuple carries an optional
// element at a different signature hands back rather than emit a mismatched struct.
func TestTupleOptionalShapeMismatchHandsBack(t *testing.T) {
	const src = `function want(t: [number, string?]): void {}
const pair: [number, string] = [1, "x"];
want(pair);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "optional element") {
		t.Fatalf("shape-mismatch handback reason = %q, want an optional-element reason", reason)
	}
}

// TestTupleParamDestructure proves an array-pattern parameter whose declared type is a
// tuple binds each name off the positional field of the held struct rather than through
// AtI: function f([a, b]: [number, string]) lowers a and b to __0.E0 and __0.E1.
func TestTupleParamDestructure(t *testing.T) {
	const src = `export function f([a, b]: [number, string]): string {
  return b + ":" + a;
}
`
	got := renderProgram(t, src)
	if !strings.Contains(got, ".E0") || !strings.Contains(got, ".E1") {
		t.Errorf("tuple-parameter destructure did not read positional fields:\n%s", got)
	}
	if strings.Contains(got, ".AtI(") {
		t.Errorf("tuple-parameter destructure should read fields, not array positions:\n%s", got)
	}
}

// TestTupleParamDestructureRuns builds and runs a tuple-parameter destructure, covering
// a heterogeneous tuple and a pattern that binds fewer names than the tuple has.
func TestTupleParamDestructureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function f([a, b]: [number, string]): string {
  return b + ":" + a;
}
console.log(f([7, "x"]));
function g([first]: [string, boolean]): string {
  return first;
}
console.log(g(["y", true]));
`
	want := "x:7\ny\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("tuple-parameter destructure printed %q, want %q", got, want)
	}
}

// TestTupleParamDestructureHandsBackOptional pins the zero-fail edge: a tuple-parameter
// bind of an optional position, whose field is a value.Opt the plain read would not peel,
// hands back rather than mislower.
func TestTupleParamDestructureHandsBackOptional(t *testing.T) {
	const src = `export function f([a, b]: [number, string?]): number {
  return a;
}
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "optional or rest element") {
		t.Fatalf("optional tuple-parameter handback reason = %q, want an optional-element reason", reason)
	}
}

// TestTupleParamNestedPattern proves a nested array pattern in a tuple parameter binds
// its inner names off the position the outer element selects: [[a, b], c]: [[number,
// string], boolean] reads the inner tuple field into a temporary, then binds a and b off
// it, the same read-into-a-temp the array-pattern binder takes.
func TestTupleParamNestedPattern(t *testing.T) {
	skipIfShort(t)
	const src = `
function f([[a, b], c]: [[number, string], boolean]): string {
  return a + b + c;
}
console.log(f([[1, "x"], true]));
`
	want := "1xtrue\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("nested tuple-parameter pattern printed %q, want %q", got, want)
	}
}

// TestTupleParamDeadDefault proves a default over a required tuple position is dead, since
// the field is always present, so the binding reads the field directly and the default
// never fires.
func TestTupleParamDeadDefault(t *testing.T) {
	skipIfShort(t)
	const src = `
function g([p, q = 9]: [number, number]): number {
  return p + q;
}
console.log(g([3, 4]) + "");
`
	want := "7\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("dead-default tuple parameter printed %q, want %q", got, want)
	}
}

// TestTupleDestructureNumberDefaultFloatifies pins that a number-literal default in
// an always-undefined tuple destructure binds x := 23.0, the float64 the number sink
// takes, rather than the bare int coerceToType leaves, which the emitted Go could not
// pass to value.Number. It mirrors the floatification a plain let x = 23 already gets.
func TestTupleDestructureNumberDefaultFloatifies(t *testing.T) {
	const src = `let [x = 23] = [undefined];
console.log(x);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "x := 23.0") {
		t.Errorf("tuple destructure number default did not floatify:\n%s", source)
	}
}
