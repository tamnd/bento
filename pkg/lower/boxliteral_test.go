package lower

import (
	"strings"
	"testing"
)

// TestBoxObjectLiteralEmitsValueObject pins the object-literal box: a literal bound
// into a dynamic slot builds value.NewObject().Set(...) per member, keyed by the
// property name, rather than interning a Go struct the any slot never names.
func TestBoxObjectLiteralEmitsValueObject(t *testing.T) {
	const src = `const o: any = { x: 1, y: "hi" };
console.log(String(o.x));
`
	source := renderProgram(t, src)
	want := `value.NewObject().Set(value.FromGoString("x"), value.Number(1)).Set(value.FromGoString("y"), value.StringValue(value.FromGoString("hi")))`
	if !strings.Contains(source, want) {
		t.Errorf("boxed object literal did not print %q:\n%s", want, source)
	}
	// The static struct path must not fire, so no shape type leaks for the any slot.
	if strings.Contains(source, "type Obj") {
		t.Errorf("boxed object literal leaked a static struct declaration:\n%s", source)
	}
}

// TestBoxArrayLiteralEmitsValueArray pins the array-literal box: a literal bound
// into a dynamic slot builds value.NewArrayValue over a []value.Value of the boxed
// elements, the dense array a boxed value carries.
func TestBoxArrayLiteralEmitsValueArray(t *testing.T) {
	const src = `const a: any = [10, 20, 30];
console.log(String(a[0]));
`
	source := renderProgram(t, src)
	want := `value.NewArrayValue([]value.Value{value.Number(10), value.Number(20), value.Number(30)})`
	if !strings.Contains(source, want) {
		t.Errorf("boxed array literal did not print %q:\n%s", want, source)
	}
}

// TestBoxLiteralRoundTrips runs the boxed object and array back through dynamic
// member and element access, so the values a JavaScript program would read come
// back out of the box unchanged.
func TestBoxLiteralRoundTrips(t *testing.T) {
	skipIfShort(t)
	const src = `const a: any = [10, 20, 30];
console.log(String(a[0]));
console.log(String(a.length));
const o: any = { x: 1, y: "hi" };
console.log(String(o.x));
console.log(String(o.y));
`
	out := runProgramGo(t, src)
	want := "10\n3\n1\nhi\n"
	if out != want {
		t.Errorf("boxed literal round trip = %q, want %q", out, want)
	}
}
