package lower

import (
	"strings"
	"testing"
)

// TestConciseArrowOverBoxedLocalHandsBack pins that a concise arrow whose body is a
// bare local forced dynamic by a computed key hands back rather than emit a wrong
// answer. The local is stored value.Value, but its checker type stays the object
// shape. Spelling that struct as the arrow's result type does not build, and spelling
// value.Value builds but leaves every consumer of the call keyed to the struct shape,
// so it re-boxes the already-boxed result with ObjectFromStruct and breaks identity:
// g() === o goes false where the runtime returns true. Threading the box through call
// sites is a dataflow slice of its own, so until then this hands back. A handback is
// always safe; a wrong answer is not. This is the shape the assignmenttargettype
// simple-complex-memberexpression test262 cases lower, which before this hand back
// emitted Go that did not build (a fail).
func TestConciseArrowOverBoxedLocalHandsBack(t *testing.T) {
	const src = "let v = 'v';\nlet o = { [v]: 1, f() {} };\nlet f = () => o;\no[v] = 1;\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "concise arrow returning a binding forced dynamic by a computed key") {
		t.Fatalf("handback reason = %q, want the boxed-concise-arrow reason", reason)
	}
}
