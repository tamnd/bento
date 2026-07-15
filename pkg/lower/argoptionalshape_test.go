package lower

import (
	"strings"
	"testing"
)

// An object literal passed where a parameter declares a fixed shape with an optional
// property must build at that shape, not its own all-required fresh type, since the
// two intern different Go structs. Before this the call site emitted a literal of the
// wrong struct and the Go build failed. These tests pin the contextual argument build
// and the handback the guard keeps for a non-literal cross-shape source.

// TestOptionalShapeArgEmptyRuns proves an empty object literal passed to a parameter
// whose shape has only optional properties builds at the parameter's shape, so each
// optional field reads as absent.
func TestOptionalShapeArgEmptyRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function h(o: {a?: number, b?: number}): number { return (o.a ?? 0) + (o.b ?? 0); }\nconsole.log(h({}));\n"
	want := "0\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("empty optional-shape arg printed %q, want %q", got, want)
	}
}

// TestOptionalShapeArgPartialRuns proves a literal that supplies one optional field
// fills it and leaves the other absent, built at the parameter's shape.
func TestOptionalShapeArgPartialRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function h(o: {a?: number, b?: number}): number { return (o.a ?? 0) + (o.b ?? 100); }\nconsole.log(h({a: 3}));\n"
	want := "103\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("partial optional-shape arg printed %q, want %q", got, want)
	}
}

// TestDestructuredParamMemberDefaultRuns proves a destructured parameter with a
// default on each member binds the defaults when the argument omits those properties,
// the item this slice unblocks: the empty literal builds at the parameter's shape and
// the body reads each optional slot, filling the default when it is absent.
func TestDestructuredParamMemberDefaultRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function f({a = 1, b = 2}: {a?: number, b?: number}): number { return a + b; }\nconsole.log(f({}));\nconsole.log(f({a: 10}));\n"
	want := "3\n12\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("destructured member default printed %q, want %q", got, want)
	}
}

// TestArrowDestructuredParamMemberDefaultRuns proves the same member-default binding
// works when the destructured parameter is on a const-bound arrow.
func TestArrowDestructuredParamMemberDefaultRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const f = ({a = 5}: {a?: number}): number => a;\nconsole.log(f({}));\nconsole.log(f({a: 9}));\n"
	want := "5\n9\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("arrow destructured member default printed %q, want %q", got, want)
	}
}

// TestOptionalShapeArgBuildsAtParamShape pins that the emitted call builds the
// argument at the parameter's struct with the optional fields, not a bare
// empty-struct literal the parameter slot would refuse.
func TestOptionalShapeArgBuildsAtParamShape(t *testing.T) {
	const src = "function h(o: {a?: number, b?: number}): number { return o.a ?? 0; }\nconsole.log(h({}));\n"
	source := renderProgram(t, src)
	if strings.Contains(source, "ObjEmpty{}") {
		t.Errorf("empty literal built at its own empty struct rather than the parameter shape:\n%s", source)
	}
	if !strings.Contains(source, "value.None") {
		t.Errorf("optional fields were not filled with value.None:\n%s", source)
	}
}

// TestCrossShapeArgHandsBack pins that a non-literal source of a different fixed shape
// with an optional property hands back rather than emit a Go struct passed for
// another, keeping the zero-fail invariant the contextual literal build cannot serve.
func TestCrossShapeArgHandsBack(t *testing.T) {
	const src = "function h(o: {a: number, b?: number}): number { return o.a; }\nconst src2: {a: number} = {a: 5};\nconsole.log(h(src2));\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "different shape with an optional property") {
		t.Fatalf("cross-shape arg handed back with %q, want the optional-shape-cross reason", reason)
	}
}
