package lower

import (
	"strings"
	"testing"
)

// new Object() builds a live property bag: a boxed value.Object the dynamic Get and
// Set paths read and write by name. A write o.k = v on such a receiver lowers to a
// Set call, a read o.k to a Get call, and a + over two reads to value.Add, so an
// untyped object composes the way JavaScript's own dynamic object does.

// TestNewObjectLowersToNewObject proves new Object() lowers to value.NewObject, the
// empty boxed object.
func TestNewObjectLowersToNewObject(t *testing.T) {
	const src = "const o: any = new Object(); o.a = 1; console.log(o.a);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewObject()") {
		t.Errorf("new Object() did not lower to value.NewObject():\n%s", source)
	}
}

// TestDynamicPropertySetLowersToSet proves a write to a property of a dynamic
// receiver lowers to a Set call that boxes the value, not a Go field assignment.
func TestDynamicPropertySetLowersToSet(t *testing.T) {
	const src = "const o: any = new Object(); o.a = 1; console.log(o.a);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, `o.Set(value.FromGoString("a"), value.Number(1))`) {
		t.Errorf("property write on a dynamic receiver did not lower to a boxed Set:\n%s", source)
	}
}

// TestDynamicObjectRuns builds and runs a program that sets and reads named
// properties, adds two boxed number properties, overwrites a property from its own
// value, and reads a missing key, so the property bag behavior is proven against
// the JavaScript result rather than just the emitted shape.
func TestDynamicObjectRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const o: any = new Object();
o.a = 1;
o.b = "hi";
console.log(o.a, o.b);

const p: any = new Object();
const q: any = new Object();
p.n = 1;
q.n = 1;
console.log(p.n + q.n);

const r: any = new Object();
r.count = 0;
r.count = r.count + 5;
console.log(r.count);
console.log(r.missing);
`
	if got, want := runProgramGo(t, src), "1 hi\n2\n5\nundefined\n"; got != want {
		t.Fatalf("dynamic object program printed %q, want %q", got, want)
	}
}

// TestNewObjectWithArgumentHandsBack proves the value form stays scoped to the empty
// constructor: new Object(x) is the ToObject coercion, a distinct later slice, so it
// hands back rather than dropping the argument.
func TestNewObjectWithArgumentHandsBack(t *testing.T) {
	const src = "const o: any = new Object(5); o.a = 1; console.log(o.a);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "ToObject") {
		t.Errorf("new Object(x) handback reason = %q, want it to name the ToObject coercion", reason)
	}
}
