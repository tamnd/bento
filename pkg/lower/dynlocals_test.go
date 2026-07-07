package lower

import (
	"strings"
	"testing"
)

// TestDynNarrowedReadEmits pins the accessor unwrap: a boxed dynamic local read
// where the checker narrowed it to one primitive, past a typeof guard, goes
// through the matching accessor, so the static expression around it takes the
// unboxed Go value. A read still typed any keeps the bare box for the runtime
// helper it flows into.
func TestDynNarrowedReadEmits(t *testing.T) {
	const src = `function f(x: any): string {
  if (typeof x === 'string') {
    return 'v=' + x;
  }
  if (typeof x === 'number') {
    return String(x * 2);
  }
  if (typeof x === 'boolean') {
    return x ? 'y' : 'n';
  }
  return String(x);
}
console.log(f("s"));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.Concat(value.FromGoString(\"v=\"), x.AsString())",
		"x.AsNumber() * 2",
		"if x.AsBool() {",
		"value.ToString(x)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("narrowed dynamic read did not print %q:\n%s", want, source)
		}
	}
}

// TestDynStringConcatEmits pins the + with a string operand over an open
// dynamic value: the result kind is known (a string operand always
// concatenates), so it lowers to Concat with the boxed side run through
// ToString rather than to the boxed value.Add, and the bstr result matches the
// string the checker types.
func TestDynStringConcatEmits(t *testing.T) {
	const src = `function g(x: any): string {
  return 'v:' + x;
}
console.log(g(7));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.Concat(value.FromGoString(\"v:\"), value.ToString(x))") {
		t.Errorf("string + dynamic did not print the ToString concat:\n%s", source)
	}
	if strings.Contains(source, "value.Add") {
		t.Errorf("string + dynamic leaked a boxed Add:\n%s", source)
	}
}

// TestDynLocalScopedPastNestedName pins that the boxed-locals pre-pass scopes each
// name to its own function. A helper declares a local named the same as a top-level
// dynamic binding, and the pre-pass used to count the two declarations as one
// redeclared name and drop the top-level binding, so its narrowed read stayed a bare
// box that double-boxed into an any parameter. The top-level read must unbox through
// its accessor and the helper's static local must stay a plain string.
func TestDynLocalScopedPastNestedName(t *testing.T) {
	const src = `function tag(v: any): string {
  var result = "<" + String(v) + ">";
  return result;
}
var result;
result = "abc".replaceAll("b", "$$");
console.log(tag(result));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "Tag(value.StringValue(result.AsString()))") {
		t.Errorf("top-level dynamic read did not unbox before the any parameter:\n%s", source)
	}
	if strings.Contains(source, "value.StringValue(result))") {
		t.Errorf("top-level dynamic read double-boxed the bare slot:\n%s", source)
	}
}

// TestDynClosureShadowsOuterName pins that a closure which redeclares an outer
// dynamic name as its own static local does not inherit the outer binding's box.
// The closure captures nothing dynamic here, so its result is a plain string read as
// a bool truthiness test, not a value accessor on a Go bool.
func TestDynClosureShadowsOuterName(t *testing.T) {
	const src = `var flag;
flag = "on";
var check = function (): string {
  var flag = "hello".length > 0;
  if (flag) {
    return "yes";
  }
  return "no";
};
console.log(flag);
console.log(check());
`
	source := renderProgram(t, src)
	if strings.Contains(source, "flag.AsBool()") {
		t.Errorf("closure static local inherited the outer dynamic box:\n%s", source)
	}
}
