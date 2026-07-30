package lower

import (
	"strings"
	"testing"
)

// TestDataCtorEmitsBagBuilder pins that a plain function used only through new whose
// body only assigns this.<name> lowers to a NewF that builds a value.Object, sets the
// field, and returns the bag, rather than a func or a handback.
func TestDataCtorEmitsBagBuilder(t *testing.T) {
	const src = "function Con() { this.x = 1; }\nvar o = new Con();\nconsole.log(o.x);\n"
	source := renderProgramTolerant(t, src)
	if !strings.Contains(source, "func NewCon() value.Value") {
		t.Fatalf("data constructor did not lower to a NewCon bag builder:\n%s", source)
	}
	if !strings.Contains(source, `Set(value.FromGoString("x"), value.Number(1))`) {
		t.Fatalf("data constructor did not set its field through the bag:\n%s", source)
	}
	if !strings.Contains(source, "NewCon()") {
		t.Fatalf("new Con() did not lower to a NewCon call:\n%s", source)
	}
}

// TestDataCtorRuns proves the bag a data constructor builds runs against the Node
// oracle: new Con() fills this.x and the binding reads it back through the value model.
func TestDataCtorRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function Con() { this.x = 1; this.y = 2; }\nvar o = new Con();\nconsole.log(o.x);\nconsole.log(o.y);\n"
	if got, want := runProgramGoTolerant(t, src), "1\n2\n"; got != want {
		t.Fatalf("data constructor run = %q, want %q", got, want)
	}
}

// TestDataCtorPlainCallStaysFunc proves a function called plainly, not only through
// new, is not treated as a data constructor: replacing its declaration with NewF would
// drop the plain call, so the scan disqualifies it and it keeps the func path.
func TestDataCtorPlainCallNotClaimed(t *testing.T) {
	const src = "function Con() { this.x = 1; }\nCon();\n"
	// A function read outside constructor position must not become a NewCon bag builder.
	source := renderProgramTolerantHandBack(t, src)
	if strings.Contains(source, "func NewCon()") {
		t.Fatalf("a plainly-called function was wrongly lowered as a data constructor:\n%s", source)
	}
}
