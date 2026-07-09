package build

import (
	"strings"
	"testing"
)

// TestArrayProtoMapCallStringLowers pins the borrowed idiom
// Array.prototype.map.call(arrayLike, String) as it appears in the test262 assert
// prelude's compareArray.format. The receiver is the function Array.prototype.map,
// not a value, so the call routes to the map-and-stringify helper rather than
// handing back at the non-string-receiver gate. The result is a dynamic-element
// array the following .join then renders, so the whole format expression lowers.
func TestArrayProtoMapCallStringLowers(t *testing.T) {
	src := "function fmt(arrayLike: any): string {\n  return \"[\" + Array.prototype.map.call(arrayLike, String).join(\", \") + \"]\";\n}\nconsole.log(fmt([1, 2, 3]));\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("Array.prototype.map.call(x, String) should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.MapCallString(") {
		t.Fatalf("expected the borrow to route through value.MapCallString, got:\n%s", out)
	}
}

// TestArrayProtoMapCallOtherCallbackHandsBack pins that admitting the String form
// did not open the borrow to an arbitrary callback: a callback that is not the
// String built-in still hands back, since lowering a general first-class callback
// through the borrow is a later slice. The worst case must stay handback, never a
// call to a helper that only knows how to stringify.
func TestArrayProtoMapCallOtherCallbackHandsBack(t *testing.T) {
	src := "function dbl(x: any): any { return x; }\nfunction m(arrayLike: any): any {\n  return Array.prototype.map.call(arrayLike, dbl);\n}\nconsole.log(m([1, 2, 3]));\n"
	if _, err := compileSource(t, src); err == nil {
		t.Fatal("Array.prototype.map.call with a non-String callback should hand back, but it lowered")
	}
}
