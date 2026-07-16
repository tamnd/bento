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

// TestTupleHandsBackOptional pins the zero-fail edge for the element forms this
// slice defers: an optional element lowers through value.Opt in a later sub-slice,
// so a tuple carrying one hands back rather than emit a partial struct.
func TestTupleHandsBackOptional(t *testing.T) {
	const src = "export function f(pair: [string, number?]): number { return pair[0].length; }\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "optional tuple element") {
		t.Fatalf("optional-element tuple handback reason = %q, want an optional-element reason", reason)
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
