package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestNestedOverloadSignatureHandsBack pins the boundary for a nested function
// overload set: the signature declarations carry no body, only the trailing
// implementation does, so the signature has nothing to lower. Before the guard,
// the body scan read the signature's nil body node and panicked with a nil
// dereference, which the ahead-of-time path must never do. The whole unit hands
// back with a named reason and runs on the engine where the overload set
// resolves.
func TestNestedOverloadSignatureHandsBack(t *testing.T) {
	const src = `function outer() {
	function inner(x: number): number;
	function inner(x: string): string;
	function inner(a: any): any { return a; }
	return inner(0);
}
outer();
`
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "overload signature") {
		t.Errorf("hand-back reason = %q, want it to mention an overload signature", nyl.Reason)
	}
}
