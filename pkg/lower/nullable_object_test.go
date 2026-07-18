package lower

import (
	"strings"
	"testing"
)

// TestNullableObjectEmitsPointerArm pins the type half of the nullable-object tagged
// sum: one plain-record object beside a null sentinel becomes a tag type, a value
// struct with a pointer field for the object arm and no field for the sentinel, a
// constructor per arm, and a JSONArm that returns the pointer or nil.
func TestNullableObjectEmitsPointerArm(t *testing.T) {
	const src = `let box: { a: number } | null = { a: 1 };
console.log(box === null);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"type ObjAOrNullTag uint8",
		"ObjAOrNullObjA ObjAOrNullTag = iota",
		"ObjAOrNullNull",
		"type ObjAOrNull struct {",
		"objA *ObjA",
		"func ObjAOrNullOfObjA(v *ObjA) ObjAOrNull",
		"func ObjAOrNullOfNull() ObjAOrNull",
		"func (u ObjAOrNull) JSONArm() any",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("emitted Go missing %q\n%s", want, source)
		}
	}
}

// TestNullableObjectConstructsFromLiteral pins that an object literal assigned to the
// union wraps in the object-arm constructor and takes the &Struct pointer, so the box
// carries the same reference an ordinary object binding does.
func TestNullableObjectConstructsFromLiteral(t *testing.T) {
	const src = `let box: { a: number } | null = { a: 1 };
box = null;
console.log(box === null);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "ObjAOrNullOfObjA(&ObjA{A: 1})") {
		t.Fatalf("object literal not wrapped in the object-arm constructor\n%s", source)
	}
	if !strings.Contains(source, "box = ObjAOrNullOfNull()") {
		t.Fatalf("null reassignment not wrapped in the null-arm constructor\n%s", source)
	}
}

// TestNullableObjectNarrows pins that box !== null lowers to a not-equal tag compare
// and the narrowed member read selects the object arm's pointer field, so box.a
// becomes box.objA.A, while box === null lowers to an equal tag compare.
func TestNullableObjectNarrows(t *testing.T) {
	const src = `let box: { a: number } | null = { a: 1 };
if (box !== null) {
  console.log(box.a);
}
if (box === null) {
  console.log("none");
}
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "if box.tag != ObjAOrNullNull {") {
		t.Fatalf("!== null did not lower to a not-equal tag compare\n%s", source)
	}
	if !strings.Contains(source, "box.objA.A") {
		t.Fatalf("narrowed read did not select the object arm pointer field\n%s", source)
	}
	if !strings.Contains(source, "if box.tag == ObjAOrNullNull {") {
		t.Fatalf("=== null did not lower to an equal tag compare\n%s", source)
	}
}

// TestNullableObjectWithUndefined pins that a { a } | null | undefined union carries
// both a null and an undefined tag-only arm beside the object pointer arm, each with
// its own tag and constructor.
func TestNullableObjectWithUndefined(t *testing.T) {
	const src = `let box: { a: number } | null | undefined = { a: 2 };
box = undefined;
box = null;
console.log(box === null);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"ObjAOrUndefOrNullObjA",
		"ObjAOrUndefOrNullUndef",
		"ObjAOrUndefOrNullNull",
		"func ObjAOrUndefOrNullOfUndef() ObjAOrUndefOrNull",
		"func ObjAOrUndefOrNullOfNull() ObjAOrUndefOrNull",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("emitted Go missing %q\n%s", want, source)
		}
	}
}

// TestNullableObjectRuns builds and runs the nullable-object program and checks its
// output matches the JavaScript semantics: the narrowed branch reads the field, the
// null reassignment clears it, and the presence test sees the change.
func TestNullableObjectRuns(t *testing.T) {
	skipIfShort(t)
	const src = `let box: { a: number } | null = { a: 7 };
if (box !== null) {
  console.log(box.a);
}
box = null;
if (box === null) {
  console.log("cleared");
}
`
	out := runProgramGo(t, src)
	if out != "7\ncleared\n" {
		t.Fatalf("unexpected output %q", out)
	}
}

// TestNullableBuiltinStaysDynamic guards the plain-record gate: re.exec()'s
// RegExpExecArray | null result is a built-in array-like beside null, not a plain
// record, so it must stay the dynamic value.Value the whole union falls back to and
// never intern a tagged sum, whose plain-struct box would drop the match array's
// behavior. The presence test reads through the dynamic value with IsNull.
func TestNullableBuiltinStaysDynamic(t *testing.T) {
	const src = `const re = /a(b+)c/;
const m = re.exec("xxabbbcyy");
if (m !== null) {
  console.log(m[0]);
}
`
	source := renderProgram(t, src)
	if strings.Contains(source, "Tag uint8") {
		t.Fatalf("RegExpExecArray | null should not intern a tagged sum\n%s", source)
	}
	if !strings.Contains(source, ".IsNull()") {
		t.Fatalf("RegExpExecArray | null should stay a dynamic value read through IsNull\n%s", source)
	}
}
