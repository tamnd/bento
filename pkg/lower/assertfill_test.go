package lower

import (
	"strings"
	"testing"
)

// TestAssertedObjectLiteralBuildsAtAssertedShape pins that an object literal
// asserted to a fixed shape builds at the asserted shape, so a required field the
// literal omits is left off the composite literal for Go to zero-fill, rather than
// interning the literal's own fresh empty shape the assertion target cannot take.
func TestAssertedObjectLiteralBuildsAtAssertedShape(t *testing.T) {
	const src = `const p = <{ id: number; name: string }>({ id: 1 });
console.log(p.id);`
	out := renderProgram(t, src)
	if strings.Contains(out, "ObjEmpty") {
		t.Fatalf("asserted literal interned the empty shape instead of the asserted one:\n%s", out)
	}
}

// TestAssertedObjectLiteralRuns builds and runs the assertion so the zero-filled
// field is proven: the omitted required name reads as the empty string Go's zero
// value gives it, and the supplied id reads back.
func TestAssertedObjectLiteralRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const p = <{ id: number; name: string }>({ id: 7 });
console.log(p.id);
console.log(p.name === "");`
	got := runProgramGo(t, src)
	want := "7\ntrue\n"
	if got != want {
		t.Fatalf("asserted-literal run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestAsExpressionObjectLiteralBuildsAtAssertedShape pins the same for the `as`
// spelling of the assertion, `({}) as { id: number }`, through the parentheses the
// assertion source carries, so both assertion forms take the zero-fill path.
func TestAsExpressionObjectLiteralBuildsAtAssertedShape(t *testing.T) {
	const src = `const p = ({}) as { id: number };
console.log(p.id);`
	out := renderProgram(t, src)
	if strings.Contains(out, "ObjEmpty") {
		t.Fatalf("as-asserted literal interned the empty shape instead of the asserted one:\n%s", out)
	}
}
