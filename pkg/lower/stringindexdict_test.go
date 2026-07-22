package lower

import (
	"strings"
	"testing"
)

// TestStringIndexDictLowersToDynamicValue pins that a pure string index signature
// object (no declared members) lowers to the dynamic value.Value box rather than a
// generated struct, since its keys are open-ended and only known at runtime. The
// parameter, the empty-literal assignment, and the argument passes must all route
// through the box.
func TestStringIndexDictLowersToDynamicValue(t *testing.T) {
	src := `function F(x: { [k: string]: string }) {}
var obj1: { [k: string]: string };
obj1 = {};
F({});
F(obj1);
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "func F(x value.Value)") {
		t.Fatalf("string index dict parameter did not lower to value.Value:\n%s", out)
	}
	if !strings.Contains(out, "var obj1 value.Value") {
		t.Fatalf("string index dict variable did not lower to value.Value:\n%s", out)
	}
	if !strings.Contains(out, "value.NewObject()") {
		t.Fatalf("empty literal assigned to a string index dict was not boxed:\n%s", out)
	}
}

// TestFixedObjectBoxesViaObjectFromStruct pins that when a concrete fixed-shape
// object value flows into a string index dict slot it is boxed through
// value.ObjectFromStruct, bridging the generated struct into the dynamic box.
func TestFixedObjectBoxesViaObjectFromStruct(t *testing.T) {
	src := `function F(x: { [k: string]: string }) {}
var obj2 = { a: "" };
F(obj2);
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.ObjectFromStruct(obj2)") {
		t.Fatalf("fixed-shape object was not boxed via ObjectFromStruct when passed to a dict slot:\n%s", out)
	}
}
