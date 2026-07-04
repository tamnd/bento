package lower

import (
	"strings"
	"testing"
)

// TestObjectGetOwnPropertyNamesEmitsNameArray pins that
// Object.getOwnPropertyNames on a fixed-shape object lowers to the same
// compile-time value.NewArray[value.BStr] of property-name literals that
// Object.keys does, since a struct shape has no non-enumerable or symbol keys
// for the two statics to differ over.
func TestObjectGetOwnPropertyNamesEmitsNameArray(t *testing.T) {
	src := `
const o = { name: "hi", age: 3 };
const ks = Object.getOwnPropertyNames(o);
console.log(ks.length);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.NewArray[value.BStr]") {
		t.Fatalf("expected a NewArray[value.BStr] of the names, got:\n%s", out)
	}
	if !strings.Contains(out, `value.FromGoString("name")`) || !strings.Contains(out, `value.FromGoString("age")`) {
		t.Fatalf("expected the property-name literals, got:\n%s", out)
	}
	if strings.Index(out, `FromGoString("name")`) > strings.Index(out, `FromGoString("age")`) {
		t.Fatalf("expected name before age in the name array, got:\n%s", out)
	}
}

// TestObjectGetOwnPropertyNamesDynamicHandsBack pins that
// Object.getOwnPropertyNames of an expression that is not a plain identifier
// hands back, the same gate Object.keys uses, since only the argument's type is
// read and a non-identifier could carry a side effect this slice would drop.
func TestObjectGetOwnPropertyNamesDynamicHandsBack(t *testing.T) {
	src := `
const ks = Object.getOwnPropertyNames({ a: 1, b: 2 });
console.log(ks.length);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "not a plain identifier") {
		t.Fatalf("expected a non-identifier handback, got: %q", reason)
	}
}

// TestObjectGetOwnPropertyNamesRuns builds and runs the emitted Go and checks
// the name list against the Node oracle: on a plain object it matches
// Object.keys, the own names in declaration order.
func TestObjectGetOwnPropertyNamesRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const o = { name: "hi", age: 3, active: true };
const ks = Object.getOwnPropertyNames(o);
console.log(o.name);
console.log(ks.length);
console.log(ks[0]);
console.log(ks[1]);
console.log(ks[2]);
`
	got := runProgramGo(t, src)
	want := "hi\n3\nname\nage\nactive\n"
	if got != want {
		t.Fatalf("Object.getOwnPropertyNames run mismatch:\n got %q\nwant %q", got, want)
	}
}
