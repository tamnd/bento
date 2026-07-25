package lower

import (
	"strings"
	"testing"
)

// TestConcatNonPrimitiveEmits pins that a string concatenation with a non-primitive
// literal operand coerces it through value.PlusToString: the object or array literal
// boxes into a live value and the value model runs ToPrimitive with the default hint
// then ToString, the protocol the + operator uses on an object.
func TestConcatNonPrimitiveEmits(t *testing.T) {
	const src = "const s = \"x\" + { a: 1 };\nconsole.log(s);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.PlusToString(") {
		t.Errorf("non-primitive concat did not route through value.PlusToString:\n%s", source)
	}
}

// TestConcatNonPrimitiveRuns builds and runs a concatenation with an object literal
// and an array literal on each side and matches Node: an object stringifies to the
// [object Object] tag and an array joins its elements with commas.
func TestConcatNonPrimitiveRuns(t *testing.T) {
	skipIfShort(t)
	const src = `console.log("x" + { a: 1 });
console.log("v=" + [1, 2, 3]);
console.log([10, 20] + "!");
`
	got := runProgramGo(t, src)
	want := "x[object Object]\nv=1,2,3\n10,20!\n"
	if got != want {
		t.Fatalf("non-primitive concat program printed %q, want %q", got, want)
	}
}

// TestConcatStructVarEmits pins that a non-primitive operand whose only form is a Go
// struct (an object-typed variable, not a literal) boxes through value.ObjectFromStruct
// and coerces with value.PlusToString, the same object concat protocol a literal operand
// takes now that the struct box constructor lands.
func TestConcatStructVarEmits(t *testing.T) {
	const src = "function f(o: { a: number }): string { return \"x\" + o; }\nconsole.log(f({ a: 1 }));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.PlusToString(value.ObjectFromStruct(o))") {
		t.Errorf("struct concat operand did not box through ObjectFromStruct + PlusToString:\n%s", source)
	}
}
