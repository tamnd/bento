package lower

import (
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// firstObjectLiteral returns the first object literal node in src, the receiver
// objectLiteralNotFixed classifies.
func firstObjectLiteral(t *testing.T, src string) (*Renderer, frontend.Node) {
	t.Helper()
	prog := compileTolerant(t, src)
	var lits []frontend.Node
	collectKind(prog, prog.SourceFiles(), frontend.NodeObjectLiteralExpression, &lits)
	if len(lits) == 0 {
		t.Fatal("no object literal in snippet")
	}
	return NewRenderer(prog), lits[0]
}

// A literal whose keys are all plain identifiers, or computed names bracketing a
// string or numeric constant, has a closed key set and stays on the struct path.
func TestObjectLiteralFixedShapes(t *testing.T) {
	fixed := []string{
		`const o = { a: 1, b: 2 };`,
		`const o = { ["c"]: 2 };`,
		`const o = { a: 1, [("b")]: 2 };`,
		`const o = { [` + "`" + `d` + "`" + `]: 3 };`,
	}
	for _, src := range fixed {
		r, lit := firstObjectLiteral(t, src)
		if r.objectLiteralNotFixed(lit) {
			t.Errorf("objectLiteralNotFixed(%q) = true, want false", src)
		}
	}
}

// A literal with a computed name over a runtime value, an identifier or a symbol,
// has no compile-time key set and must build as the dynamic bag.
func TestObjectLiteralNotFixedShapes(t *testing.T) {
	notFixed := []string{
		`let k = "x"; const o = { [k]: 1 };`,
		`const s = Symbol(); const o = { [s]: 1 };`,
		`let k = "x"; const o = { a: 1, [k]: 2 };`,
	}
	for _, src := range notFixed {
		r, lit := firstObjectLiteral(t, src)
		if !r.objectLiteralNotFixed(lit) {
			t.Errorf("objectLiteralNotFixed(%q) = false, want true", src)
		}
	}
}
