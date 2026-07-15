package lower

import (
	"strings"
	"testing"
)

// The abstract-equality rule that coerces before it compares runs whenever a
// loose == or != joins two static primitives the operator table cannot fold: a
// number against a string, a boolean against a number, or any pair whose kinds
// differ. staticPrimitiveEquality routes those through value.LooseEquals, so the
// comparison coerces the way the language specifies rather than folding to Go's
// EQL over mismatched machine types. The checker judges these comparisons to
// have no type overlap and raises 2367, which the AOT front door tolerates
// because the equality still runs, so these cases drive the tolerant path.

func TestNumberEqualsStringCoerces(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `console.log(1 == "1");`); got != "true\n" {
		t.Fatalf("1 == \"1\" = %q, want true", got)
	}
}

func TestNumberNotEqualsStringCoerces(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `console.log(1 != "1");`); got != "false\n" {
		t.Fatalf("1 != \"1\" = %q, want false", got)
	}
}

func TestBoolEqualsNumberCoerces(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `console.log(true == 1);`); got != "true\n" {
		t.Fatalf("true == 1 = %q, want true", got)
	}
}

func TestZeroEqualsEmptyStringCoerces(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `console.log(0 == "");`); got != "true\n" {
		t.Fatalf("0 == \"\" = %q, want true", got)
	}
}

func TestStringEqualsStringNoOverlapCompares(t *testing.T) {
	skipIfShort(t)
	// Two distinct string literals draw 2367 as well, since each carries its own
	// literal type; the comparison still runs and returns false.
	if got := runProgramGoTolerant(t, `console.log("a" == "b");`); got != "false\n" {
		t.Fatalf("\"a\" == \"b\" = %q, want false", got)
	}
}

func TestStaticPrimitiveEqualityRoutesThroughLooseEquals(t *testing.T) {
	src := `console.log(1 == "1");`
	got := renderProgramTolerant(t, src)
	if !strings.Contains(got, "value.LooseEquals") {
		t.Fatalf("emit did not route through value.LooseEquals:\n%s", got)
	}
}
