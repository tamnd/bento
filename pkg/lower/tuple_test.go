package lower

import (
	"strings"
	"testing"
)

// TestTupleTypeHandsBack pins the zero-fail edge of the tuple facts slice: a
// tuple type reaches typeExpr now that the partitioner treats it as lowerable,
// and until the positional-struct emission slice lands it hands back with a
// tuple-specific reason rather than falling through to renderObject, which would
// intern the tuple's inherited array members as struct fields and miscompile the
// shape (typed/05 delivery slice 1).
func TestTupleTypeHandsBack(t *testing.T) {
	const src = "export function first(pair: [string, number]): number { return pair[1]; }\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "tuple") {
		t.Fatalf("tuple type handback reason = %q, want a tuple-specific handback", reason)
	}
}
