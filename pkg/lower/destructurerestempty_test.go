package lower

import (
	"strings"
	"testing"
)

// TestEmptyRestGathersARuntimeObject pins the rule. The empty object type is the
// structural top type everywhere else in the lowerer, so typeExpr gives it a value.Value
// slot and a `const e = {}` builds a value.NewObject. A rest with nothing left in it is
// that same type, so it builds that same object rather than an interned empty struct.
func TestEmptyRestGathersARuntimeObject(t *testing.T) {
	const src = "const src = { a: 1 };\n" +
		"const { a, ...rest } = src;\n" +
		"console.log(a, Object.keys(rest).length);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "rest := value.NewObject()") {
		t.Errorf("the empty rest did not gather a runtime object:\n%s", got)
	}
	if strings.Contains(got, "ObjEmpty") {
		t.Errorf("the empty rest still interned a struct:\n%s", got)
	}
}

// TestEmptyRestReadsThroughTheValueModel pins why this was a build break rather than a
// wrong answer. isDynamic routes every read of the empty object type through the runtime,
// so the read was already emitted as OwnEnumerableKeys while the bind handed it a struct
// that has no such method.
func TestEmptyRestReadsThroughTheValueModel(t *testing.T) {
	const src = "const src = { a: 1 };\n" +
		"const { a, ...rest } = src;\n" +
		"console.log(a, Object.keys(rest).length);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "rest.OwnEnumerableKeys()") {
		t.Errorf("the read off the empty rest left the value model:\n%s", got)
	}
}

// TestEmptyRestHoistsAsABox pins the other side of the same disagreement. A leaf a
// function reads hoists to a package var typeExpr spells, which for the empty object type
// is a value.Value, and the bind that stays in main has to store one into it. It stored
// &ObjEmpty{} and the Go refused the assignment.
func TestEmptyRestHoistsAsABox(t *testing.T) {
	const src = "const src = { a: 1 };\n" +
		"const { a, ...rest } = src;\n" +
		"function f(): number { return a + Object.keys(rest).length; }\n" +
		"console.log(f());\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "rest = value.NewObject()") {
		t.Errorf("the hoisted empty rest did not store a box:\n%s", got)
	}
	if strings.Contains(got, "&ObjEmpty{}") {
		t.Errorf("the hoisted empty rest still stored a struct:\n%s", got)
	}
}

// TestEmptyRestAssignmentGathersARuntimeObject pins that the assignment form answers the
// same way as the declaration form. Both classify a rest and both build its gather, so
// both ask the same question of the shape.
func TestEmptyRestAssignmentGathersARuntimeObject(t *testing.T) {
	const src = "const src = { a: 1, b: 2 };\n" +
		"let ra = 0, rb = 0;\n" +
		"let rest: {} = {};\n" +
		"({ a: ra, b: rb, ...rest } = src);\n" +
		"console.log(ra, rb, Object.keys(rest).length);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "value.NewObject()") {
		t.Errorf("the empty assignment rest did not gather a runtime object:\n%s", got)
	}
	if strings.Contains(got, "ObjEmpty") {
		t.Errorf("the empty assignment rest still interned a struct:\n%s", got)
	}
}

// TestRestWithSomethingLeftKeepsItsStruct pins the boundary. A rest that still names
// properties is a fixed shape like any other, so it interns and copies field by field,
// which is what makes Object.keys of it fold to the compile-time key list.
func TestRestWithSomethingLeftKeepsItsStruct(t *testing.T) {
	const src = "const src = { a: 1, b: 2, c: 3 };\n" +
		"const { a, ...rest } = src;\n" +
		"console.log(a, Object.keys(rest).length);\n"
	got := renderProgram(t, src)
	if strings.Contains(got, "rest := value.NewObject()") {
		t.Errorf("a rest with properties left in it gathered a runtime object:\n%s", got)
	}
	if !strings.Contains(got, "rest := &ObjBC{B: src.B, C: src.C}") {
		t.Errorf("the rest did not gather its own struct:\n%s", got)
	}
}
