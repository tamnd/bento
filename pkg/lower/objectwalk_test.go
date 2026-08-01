package lower

import (
	"strings"
	"testing"
)

// TestAnObjectWalkOverATypedReceiverBoxesItsAnswer pins the emit. The walk hands back a
// bag of boxes, and where the checker has a value type for the receiver it types the
// call after it, so the two disagree and the bag is boxed whole rather than handed to a
// consumer that asked for an array of structs.
func TestAnObjectWalkOverATypedReceiverBoxesItsAnswer(t *testing.T) {
	const src = "type Row = { id: number };\n" +
		"const m = JSON.parse('{}') as Record<string, Row>;\n" +
		"console.log(Object.values(m).length);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "OwnValues().ToValue()") {
		t.Fatalf("a walk over a typed record did not box its answer:\n%s", out)
	}
}

// TestAnObjectWalkOverAnUntypedReceiverKeepsItsArray is the boundary in the other
// direction. A receiver the checker has no value type for gives the call any[], which a
// bag of boxes is, so nothing moves and the golden stays where it was.
func TestAnObjectWalkOverAnUntypedReceiverKeepsItsArray(t *testing.T) {
	const src = "const o: any = { a: 1, b: 2 };\nconsole.log(Object.values(o).length);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "OwnValues()") || strings.Contains(out, "OwnValues().ToValue()") {
		t.Fatalf("a walk over an untyped receiver did not keep its array:\n%s", out)
	}
}

// TestAnIndexReadOffABoxedPairDispatchesAtRunTime pins the tuple case. The callback
// holds a box while the checker calls it a [string, Row], and a tuple's positions are
// fields only when the tuple is a Go struct, which this one is not.
func TestAnIndexReadOffABoxedPairDispatchesAtRunTime(t *testing.T) {
	const src = "type Row = { id: number };\n" +
		"const m = JSON.parse('{}') as Record<string, Row>;\n" +
		"console.log(Object.entries(m).map((e: [string, Row]) => e[0]));"
	out := renderProgram(t, src)
	if strings.Contains(out, "e.E0") {
		t.Fatalf("an index read off a boxed pair selected a tuple field:\n%s", out)
	}
	if !strings.Contains(out, "GetIndex(") {
		t.Fatalf("an index read off a boxed pair did not dispatch at run time:\n%s", out)
	}
}

// TestACallOffABoxedChainKeepsItsBox pins where a chain stops being boxed. A read the
// checker types number is coerced down to a float64 at the read itself, so the chain
// ends there; a call dispatched through the runtime is not, so it hands back a box
// whatever the checker calls the result.
func TestACallOffABoxedChainKeepsItsBox(t *testing.T) {
	const src = "const nums = JSON.parse('{}') as Record<string, number>;\n" +
		"console.log(Object.values(nums).reduce((a: number, b: number) => a + b, 0));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.ConsoleValue(") {
		t.Fatalf("a call off a boxed chain did not keep its box:\n%s", out)
	}
}
