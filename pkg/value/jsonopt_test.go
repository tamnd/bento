package value

import "testing"

// JSON.stringify of a top-level optional unwraps the option before the walk. Without
// that the walk reaches the value.Opt struct itself, whose two fields are unexported,
// and writes it as an empty object.

// TestJSONStringifyOfAnOptionalUnwrapsIt holds both arms. The present one serializes
// what it holds, and the absent one is the value undefined rather than a string, the
// same result JSON.stringify(undefined) has in Node.
func TestJSONStringifyOfAnOptionalUnwrapsIt(t *testing.T) {
	present := JSONStringifyOpt(Some(newPoint()))
	if got := present.AsString().ToGoString(); got != `{"x":1,"y":"s"}` {
		t.Errorf("a present optional serialized as %s", got)
	}

	absent := JSONStringifyOpt(None[*pointClass]())
	if absent.Kind() != KindUndefined {
		t.Errorf("an absent optional serialized as %v, want undefined", NodeInspect(absent).ToGoString())
	}

	// A value that is not an option at all passes straight through, so the helper is
	// safe on the shapes the lowering's predicate does not promise anything about.
	if got := JSONStringifyOpt(Number(3).AsNumber()).AsString().ToGoString(); got != "3" {
		t.Errorf("a plain number serialized as %s, want 3", got)
	}
}
