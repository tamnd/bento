package lower

import (
	"strings"
	"testing"
)

// A bracket read with a constant string key on an empty-object receiver must dispatch
// through the runtime Get, not fold to value.MissingProperty. The object is the
// structural top type, a value.NewObject bag whose property was written at runtime, so
// folding the read to the undefined singleton off the static shape answers the wrong
// value. The dotted read already dispatched dynamically; this pins the bracket read to
// the same Get so obj["k"] sees what obj.k = v wrote. It renders through the tolerant
// front door because the checker flags the out-of-shape read the harness admits.
func TestBracketReadOnEmptyObjectUsesGet(t *testing.T) {
	const src = "var obj = {};\nobj.k = 42;\nconsole.log(obj[\"k\"]);\n"
	out := renderProgramTolerant(t, src)
	if strings.Contains(out, "MissingProperty") {
		t.Fatalf("bracket read folded to MissingProperty instead of a runtime Get:\n%s", out)
	}
	if !strings.Contains(out, "obj.Get(value.FromGoString(\"k\"))") {
		t.Fatalf("bracket read did not dispatch through Get:\n%s", out)
	}
}

// The same shape runs and reads back the value the write stored, where the fold to
// undefined previously printed a wrong result.
func TestBracketReadOnEmptyObjectRuns(t *testing.T) {
	skipIfShort(t)
	const src = "var obj = {};\nobj.k = 42;\nconsole.log(obj[\"k\"] === 42);\n"
	if got, want := runProgramGoTolerant(t, src), "true\n"; got != want {
		t.Fatalf("empty-object bracket read printed %q, want %q", got, want)
	}
}
