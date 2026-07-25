package lower

import (
	"strings"
	"testing"
)

// A for...in head enumerates the receiver's own then inherited enumerable string
// keys. A dynamic (any) receiver lowers to a value.Value, which carries the
// ForInKeys method the range loop drives, so a plain for...in over it lowers. A
// statically-shaped receiver lowers to a Go struct or slice with no such method, so
// it hands back until a typed enumeration is modeled. A destructuring for...in head,
// the group 5 item, sits on top of the plain form: the checker rejects a
// destructuring pattern in a for...in head outright ("The left-hand side of a
// 'for...in' statement cannot be a destructuring pattern"), so it never reaches the
// lowerer under the current front door.

// TestForInDynamicLowers proves a plain for...in over a dynamic object lowers to a
// range over ForInKeys, the enumeration the harness includes need.
func TestForInDynamicLowers(t *testing.T) {
	const src = "const o: any = { a: 1 };\nfor (const k in o) {\n  console.log(k);\n}\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	p, err := r.RenderProgram(entryFile(t, prog))
	if err != nil {
		t.Fatalf("for...in over a dynamic object handed back, want a lowering: %v", err)
	}
	if !strings.Contains(p.Source, "ForInKeys()") {
		t.Fatalf("lowered for...in does not range ForInKeys:\n%s", p.Source)
	}
}

// TestForInStringIndexLowers proves a for...in over a string-index dictionary lowers:
// the dictionary boxes into a dynamic value.Value, which carries ForInKeys, so the
// enumeration ranges it the same way a bare any receiver does.
func TestForInStringIndexLowers(t *testing.T) {
	const src = "const o: { [k: string]: number } = { a: 1 };\nfor (const k in o) {\n  console.log(k);\n}\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	p, err := r.RenderProgram(entryFile(t, prog))
	if err != nil {
		t.Fatalf("for...in over a string-index dictionary handed back, want a lowering: %v", err)
	}
	if !strings.Contains(p.Source, "ForInKeys()") {
		t.Fatalf("lowered for...in does not range ForInKeys:\n%s", p.Source)
	}
}

// TestForInFixedShapeHandsBack proves a for...in over a fixed-shape object, which
// lowers to a Go struct with no ForInKeys method, still hands back, the boundary the
// dynamic-only enumeration draws once a string-index dictionary boxes.
// TestForInFixedShapeLowers pins that for...in over a plain fixed-shape object folds
// its own enumerable field names into a literal key array in the order Object.keys
// gives, ranges over that array, and discards the receiver once so the unused-local
// pass leaves it used the same way the source uses it.
func TestForInFixedShapeLowers(t *testing.T) {
	const src = "const o = { a: 1, b: 2, c: 3 };\nfor (const k in o) {\n  console.log(k);\n}\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, `range value.NewArray[value.BStr](value.FromGoString("a"), value.FromGoString("b"), value.FromGoString("c")).Elems()`) {
		t.Fatalf("for...in over a fixed shape did not fold the field names into a key array:\n%s", source)
	}
	if !strings.Contains(source, "_ = o") {
		t.Fatalf("for...in over a fixed shape did not discard the receiver to keep it used:\n%s", source)
	}
}

// TestForInClassInstanceHandsBack pins that a class instance still hands back: its
// methods live on the prototype and are not visited by for...in, but the shape's
// property list carries them, so folding would enumerate a method name the runtime
// never would.
func TestForInClassInstanceHandsBack(t *testing.T) {
	const src = "class A { x = 1; y = 2; greet(): string { return \"hi\"; } }\nconst a = new A();\nfor (const k in a) {\n  console.log(k);\n}\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("for...in over a class instance lowered, want a hand-back:\n%s", src)
	}
	if !strings.Contains(err.Error(), "later slice") {
		t.Fatalf("for...in handback reason = %q, want a later-slice deferral", err.Error())
	}
}

// TestForInFixedShapeRuns builds and runs for...in over fixed shapes and matches Node:
// the keys come in declaration order, an unread binding still drives the loop, and the
// key drives a body that reads the receiver by the same key.
func TestForInFixedShapeRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const o = { a: 1, b: 2, c: 3 };
for (const k in o) console.log(k);
let n = 0;
for (const _k in o) n++;
console.log("count", n);
const p = { x: 10, y: 20 };
const keys: string[] = [];
for (const k in p) keys.push(k);
console.log(keys.join(","));
`
	got := runProgramGo(t, src)
	want := "a\nb\nc\ncount 3\nx,y\n"
	if got != want {
		t.Fatalf("for...in fixed-shape program printed %q, want %q", got, want)
	}
}
