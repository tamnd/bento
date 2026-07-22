package lower

import (
	"strings"
	"testing"
)

// A loose == or != where one side is statically null or undefined follows the
// abstract equality rule that null and undefined equal only each other and coerce
// against nothing else. These tests pin the forms value.LooseEquals answers through
// the nullishLooseEquality path: the two nullish literals against each other, a
// nullish literal against a primitive, the side-effect order the boxing keeps, the
// emit that proves the route, and the handback a static object binding keeps until
// its box lands.

// TestNullEqualsUndefinedRuns proves the two nullish literals are loosely equal to
// each other, so null == undefined is true and null != undefined is false.
func TestNullEqualsUndefinedRuns(t *testing.T) {
	skipIfShort(t)
	const src = "console.log(null == undefined);\nconsole.log(null != undefined);\nconsole.log(undefined == null);\n"
	want := "true\nfalse\ntrue\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("null/undefined equality printed %q, want %q", got, want)
	}
}

// TestNullEqualsItselfRuns proves each nullish literal is loosely equal to its own
// kind, so null == null and undefined == undefined are both true.
func TestNullEqualsItselfRuns(t *testing.T) {
	skipIfShort(t)
	const src = "console.log(null == null);\nconsole.log(undefined == undefined);\n"
	want := "true\ntrue\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("nullish self-equality printed %q, want %q", got, want)
	}
}

// TestNullEqualsPrimitiveRuns proves a nullish literal is loosely unequal to every
// non-nullish primitive with no coercion, so null == 0 and null == "x" are false
// while undefined != 0 is true.
func TestNullEqualsPrimitiveRuns(t *testing.T) {
	skipIfShort(t)
	const src = "console.log(null == 0);\nconsole.log(null == \"x\");\nconsole.log(undefined != 0);\n"
	want := "false\nfalse\ntrue\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("nullish vs primitive printed %q, want %q", got, want)
	}
}

// TestNullEqualsKeepsSideEffect proves the non-nullish operand still evaluates, so
// null == f() calls f before comparing rather than folding to a constant that would
// drop the call.
func TestNullEqualsKeepsSideEffect(t *testing.T) {
	skipIfShort(t)
	const src = "function f(): number { console.log(\"called\"); return 3; }\nconsole.log(null == f());\n"
	want := "called\nfalse\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("null == f() printed %q, want %q", got, want)
	}
}

// TestNullEqualsRoutesThroughLooseEquals pins that a static-nullish loose equality
// emits value.LooseEquals, the coercing compare, rather than a folded constant or a
// bare Go operator the operand kinds would reject.
func TestNullEqualsRoutesThroughLooseEquals(t *testing.T) {
	const src = "console.log(null == 0);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.LooseEquals") {
		t.Fatalf("nullish loose equality did not route through value.LooseEquals:\n%s", source)
	}
}

// TestNullEqualsStaticObjectEmits pins that a static fixed-shape object binding boxes
// through value.ObjectFromStruct into the loose compare now that object boxing lands:
// an object is never nullish, so the runtime answers false, but the route is the same
// value.LooseEquals a dynamic operand takes.
func TestNullEqualsStaticObjectEmits(t *testing.T) {
	const src = "const o = { a: 1 };\nconsole.log(o == null);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.LooseEquals(value.ObjectFromStruct(o), value.Null)") {
		t.Fatalf("static object == null did not box through ObjectFromStruct into LooseEquals:\n%s", source)
	}
}
