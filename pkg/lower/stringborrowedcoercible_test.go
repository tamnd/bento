package lower

import (
	"strings"
	"testing"
)

// TestStringBorrowedCoerciblEmits pins that a borrowed String.prototype method,
// String.prototype.<m>.call(recv, ...), coerces its receiver through
// value.CoerceThisToString rather than a bare value.ToString, so a null or
// undefined receiver runs RequireObjectCoercible and throws a TypeError before the
// method body instead of stringifying to "null"/"undefined".
func TestStringBorrowedCoercibleEmits(t *testing.T) {
	const src = `const n = String.prototype.codePointAt.call("ab", 0);
console.log(n !== undefined ? n : -1);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "CoerceThisToString") {
		t.Errorf("borrowed String.prototype.codePointAt.call did not coerce through CoerceThisToString:\n%s", source)
	}
	if strings.Contains(source, "value.ToString(") {
		t.Errorf("borrowed String.prototype method still uses a bare value.ToString:\n%s", source)
	}
}

// TestStringBorrowedCoercibleThrowsOnNullish builds and runs the emitted Go to
// confirm the borrowed method throws a TypeError when its receiver is null,
// the RequireObjectCoercible step String.prototype.codePointAt.call(null) raises.
func TestStringBorrowedCoercibleThrowsOnNullish(t *testing.T) {
	skipIfShort(t)
	const src = `let threw = false;
try {
  String.prototype.codePointAt.call(null, 0);
} catch (e) {
  threw = e instanceof TypeError;
}
console.log(threw);
`
	got := runProgramGo(t, src)
	if got != "true\n" {
		t.Errorf("borrowed codePointAt on a null receiver printed %q, want %q", got, "true\n")
	}
}
