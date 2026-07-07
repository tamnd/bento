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
