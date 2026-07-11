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

// TestArrayProtoMapCallOtherCallbackLowers pins that the generic-receiver map (08
// group 1) opened the borrow to an arbitrary callback: a callback that is not the
// String built-in now routes through value.GenericMap, which boxes the callback and
// calls it with each element, its index, and the receiver. The String form still
// wins its own faster path, so only the general callback reaches GenericMap.
func TestArrayProtoMapCallOtherCallbackLowers(t *testing.T) {
	src := "function dbl(x: any): any { return x; }\nfunction m(arrayLike: any): any {\n  return Array.prototype.map.call(arrayLike, dbl);\n}\nconsole.log(m([1, 2, 3]));\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("Array.prototype.map.call with a general callback should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.GenericMap(") {
		t.Fatalf("expected the borrow to route through value.GenericMap, got:\n%s", out)
	}
}
