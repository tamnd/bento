package lower

import (
	"strings"
	"testing"
)

// An object literal with a runtime computed key builds as the dynamic bag
// (value.NewObject().SetKeyed) and its binding is marked dynBound, so a later
// string-key read off it must dispatch through the runtime Get rather than select a
// Go struct field the value.Value box does not carry. This pins the fix for the
// computed-property-names case where object["1.2"] wrongly lowered to a struct-field
// selector while object[Infinity] already lowered to the dynamic accessor.
func TestDynamicObjectStringKeyReadIsDynamic(t *testing.T) {
	const src = `let k = "x";
var o = { [k]: "A", b: "B" };
console.log(o["b"]);
`
	source := renderTolerant(t, src)
	if !strings.Contains(source, "value.NewObject().SetKeyed") {
		t.Fatalf("dynamic object literal did not build as the runtime bag:\n%s", source)
	}
	if !strings.Contains(source, `o.Get(value.FromGoString("b"))`) {
		t.Errorf("string-key read off the dynamic object did not lower to the runtime Get:\n%s", source)
	}
	if strings.Contains(source, "o.B") {
		t.Errorf("string-key read off the dynamic object wrongly lowered to a struct-field selector:\n%s", source)
	}
}

// A control: an all-identifier-key literal has a closed key set, builds as a Go
// struct, and its binding keeps the struct type, so the same string-key read still
// lowers to the fast struct-field selector. This guards the fix from loosening the
// static path.
func TestStaticObjectStringKeyReadIsFieldSelector(t *testing.T) {
	const src = `var o = { a: "A", b: "B" };
console.log(o["b"]);
`
	source := renderTolerant(t, src)
	if !strings.Contains(source, "o.B") {
		t.Errorf("string-key read off the static object did not lower to a struct-field selector:\n%s", source)
	}
	if strings.Contains(source, `o.Get(`) {
		t.Errorf("string-key read off the static object wrongly took the dynamic Get path:\n%s", source)
	}
}
