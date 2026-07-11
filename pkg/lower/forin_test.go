package lower

import (
	"strings"
	"testing"
)

// A for...in head over an object enumerates the object's own enumerable keys, which
// the AOT object model cannot enumerate yet (it is the phase 7 object work), so a
// plain for...in hands back rather than emit a partial enumeration. A destructuring
// for...in head, the group 5 item, sits on top of that: the checker rejects a
// destructuring pattern in a for...in head outright ("The left-hand side of a
// 'for...in' statement cannot be a destructuring pattern"), so it never reaches the
// lowerer under the current front door, and even a valid key binding would need the
// enumeration the plain form still lacks. So the destructuring head defers with the
// plain form, and this test pins the plain-form handback that gates it.

// TestForInHandsBack proves a plain for...in over an object hands back, the
// prerequisite the destructuring head waits on.
func TestForInHandsBack(t *testing.T) {
	const src = "const o: { [k: string]: number } = { a: 1 };\nfor (const k in o) {\n  console.log(k);\n}\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("for...in lowered, want a hand-back:\n%s", src)
	}
	if !strings.Contains(err.Error(), "later slice") {
		t.Fatalf("for...in handback reason = %q, want a later-slice deferral", err.Error())
	}
}
