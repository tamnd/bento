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
func TestForInFixedShapeHandsBack(t *testing.T) {
	const src = "const o: { a: number } = { a: 1 };\nfor (const k in o) {\n  console.log(k);\n}\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("for...in over a fixed-shape object lowered, want a hand-back:\n%s", src)
	}
	if !strings.Contains(err.Error(), "later slice") {
		t.Fatalf("for...in handback reason = %q, want a later-slice deferral", err.Error())
	}
}
