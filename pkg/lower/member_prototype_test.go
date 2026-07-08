package lower

import (
	"strings"
	"testing"
)

// TestAmbientFunctionPrototypeFoldsUndefined pins that a read of .prototype on a
// built-in function that is not a constructor lowers to the undefined singleton.
// isFinite and its siblings carry no prototype property, and bento models them as
// call targets with no first-class Go value, so a naive lowering emits a selector
// on a Go type name that does not build. The fold answers value.Undefined, the
// value the language gives.
func TestAmbientFunctionPrototypeFoldsUndefined(t *testing.T) {
	for _, fn := range []string{"isFinite", "isNaN", "parseInt", "parseFloat"} {
		src := "console.log(" + fn + ".prototype === undefined);"
		out := renderProgram(t, src)
		if !strings.Contains(out, "value.Undefined") {
			t.Fatalf("%s.prototype did not fold to value.Undefined:\n%s", fn, out)
		}
		if strings.Contains(out, ".Prototype") {
			t.Fatalf("%s.prototype kept a Go selector that does not build:\n%s", fn, out)
		}
	}
}

// TestConstructorPrototypeDoesNotFold pins the fold stays off a real constructor's
// prototype. Number.prototype is a genuine object, so the read must not fold to the
// undefined singleton; the receiver hands back to a later slice instead.
func TestConstructorPrototypeDoesNotFold(t *testing.T) {
	src := "console.log(Number.prototype === undefined);"
	reason := renderProgramHandBack(t, src)
	if reason == "" {
		t.Fatalf("Number.prototype did not hand back, it must not fold to undefined")
	}
}

// TestAmbientFunctionPrototypeRuns builds and runs the prototype reads so the
// folded undefined compares equal to undefined the way JavaScript reports it.
func TestAmbientFunctionPrototypeRuns(t *testing.T) {
	skipIfShort(t)
	src := `
console.log(isFinite.prototype === undefined);
console.log(isNaN.prototype === undefined);
console.log(parseInt.prototype === undefined);
console.log(parseFloat.prototype === undefined);
`
	got := runProgramGo(t, src)
	want := "true\ntrue\ntrue\ntrue\n"
	if got != want {
		t.Fatalf("ambient function prototype run mismatch:\n got %q\nwant %q", got, want)
	}
}
