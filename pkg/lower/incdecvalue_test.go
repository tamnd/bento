package lower

import (
	"strings"
	"testing"
)

// A ++ or -- read for its value, not run as a statement on its own line, has no
// direct Go form: Go's IncDecStmt is a statement and yields nothing. The value
// form rides an immediately-called closure over the local, so const r = ++n reads
// the incremented value and const r = n++ reads the value from before the update.

// TestPrefixIncrementValueReturnsNewValue proves ++n in value position lowers to a
// closure that increments first and returns the new value.
func TestPrefixIncrementValueReturnsNewValue(t *testing.T) {
	const src = "let n = 0; const r = ++n; console.log(r, n);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func() float64 {\n\t\tn++\n\t\treturn n\n\t}()") {
		t.Errorf("prefix ++ in value position did not lower to an increment-then-return closure:\n%s", source)
	}
}

// TestPostfixIncrementValueReturnsOldValue proves n++ in value position lowers to a
// closure that saves the old value, increments, and returns the saved one.
func TestPostfixIncrementValueReturnsOldValue(t *testing.T) {
	const src = "let n = 5; const r = n++; console.log(r, n);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "prev := n\n\t\tn++\n\t\treturn prev") {
		t.Errorf("postfix ++ in value position did not save the pre-update value:\n%s", source)
	}
}

// TestIncrementValueFormsRun builds and runs every value-position increment and
// decrement form, including one inside an index expression, so the read timing is
// proven against the JavaScript result rather than just the emitted shape.
func TestIncrementValueFormsRun(t *testing.T) {
	skipIfShort(t)
	const src = `
let n = 0;
const a = ++n;
const b = n++;
const c = n--;
const d = --n;
console.log(a, b, c, d, n);

const arr = [10, 20, 30];
let i = 0;
const first = arr[i++];
const second = arr[i++];
console.log(first, second, i);
`
	if got, want := runProgramGo(t, src), "1 1 2 0 0\n10 20 2\n"; got != want {
		t.Fatalf("value-position increment printed %q, want %q", got, want)
	}
}

// TestPrefixIncrementOnNonIdentifierHandsBack proves the value form stays scoped to
// a plain local: a ++ on a property target has no capturing closure yet and hands
// back rather than emit wrong Go.
func TestPrefixIncrementOnNonIdentifierHandsBack(t *testing.T) {
	const src = "const o = { n: 0 }; const r = ++o.n; console.log(r);\n"
	renderProgramHandBack(t, src)
}
