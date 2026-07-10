package lower

import (
	"strings"
	"testing"
)

// A module-level binding a top-level function reads hoists to a package var, and
// its initializer must be safe to build at package-init time. An object literal
// qualifies: its property names are fixed labels, not binding reads, so a literal
// whose values are constants carries no init-order dependency and no side effect.
// Before, the property name looked like an unused variable read and forced a
// handback; now the literal hoists the way an array literal already does.

// TestHoistedObjectLiteralBindingRendersAsPackageVar proves the object-literal
// binding a function reads becomes a package-level var rather than handing back.
func TestHoistedObjectLiteralBindingRendersAsPackageVar(t *testing.T) {
	const src = "var obj = { foo: 1 };\nfunction get() { return obj.foo; }\nconsole.log(get());\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "var obj ") {
		t.Errorf("object-literal module binding did not hoist to a package var:\n%s", source)
	}
	if strings.Contains(source, "func main()") {
		// The binding must live outside main; a var declared inside main would not be
		// visible to the hoisted get.
		mainIdx := strings.Index(source, "func main()")
		if strings.Contains(source[mainIdx:], "obj ") && strings.Contains(source[mainIdx:], ":=") {
			t.Errorf("object-literal binding stayed a main local:\n%s", source)
		}
	}
}

// TestHoistedObjectLiteralRuns builds and runs a module object literal read by a
// function, so the hoist is proven by the JavaScript result, not just the shape. A
// nested literal and a second property show the whole tree hoists.
func TestHoistedObjectLiteralRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
var config = { width: 4, height: 3, meta: { scale: 2 } };
function area() { return config.width * config.height * config.meta.scale; }
console.log(area());
`
	if got, want := runProgramGo(t, src), "24\n"; got != want {
		t.Fatalf("hoisted object literal printed %q, want %q", got, want)
	}
}

// TestHoistedArrayOfObjectsRuns proves an array of object literals, a common
// fixture-table shape, hoists and reads back through a function.
func TestHoistedArrayOfObjectsRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
var rows = [{ n: 10 }, { n: 20 }, { n: 30 }];
function total() {
  var s = 0;
  for (var i = 0; i < rows.length; i++) { s = s + rows[i].n; }
  return s;
}
console.log(total());
`
	if got, want := runProgramGo(t, src), "60\n"; got != want {
		t.Fatalf("hoisted array of object literals printed %q, want %q", got, want)
	}
}

// TestHoistedShorthandObjectRuns proves a shorthand { x } binding a function reads
// hoists by in-place assignment: its initializer reads the outer binding x, so it is
// not safe at package-init time, but written at its source position in main it reads
// x after x is set, and the function sees the settled value through the zero-valued
// package var. The read is of an earlier binding, so the source order is preserved.
func TestHoistedShorthandObjectRuns(t *testing.T) {
	skipIfShort(t)
	const src = "var x = 5;\nvar wrap = { x };\nfunction get() { return wrap.x; }\nconsole.log(get());\n"
	if got, want := runProgramGo(t, src), "5\n"; got != want {
		t.Fatalf("hoisted shorthand object printed %q, want %q", got, want)
	}
}
