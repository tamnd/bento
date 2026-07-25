package lower

import (
	"strings"
	"testing"
)

// The + operator's string-concatenation branch coerces a non-string operand with the
// default ToPrimitive hint, so an object concatenated to a string lowers to
// value.PlusToString, not value.ToString. The two differ for a hint-sensitive object
// (valueOf vs toString), and + must ask for the default hint.
func TestStringPlusObjectEmitsPlusToString(t *testing.T) {
	src := "const o: any = { valueOf() { return 42; } };\nconsole.log(\"\" + o);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.PlusToString(") {
		t.Errorf("string + object did not lower to value.PlusToString:\n%s", source)
	}
	if strings.Contains(source, "value.ToString(o)") {
		t.Errorf("string + object still emits the string-hint value.ToString:\n%s", source)
	}
}
