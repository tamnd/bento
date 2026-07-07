package lower

import (
	"strings"
	"testing"
)

// TestErrorConstructorBoxesToValue pins that a built-in error constructor named in
// a dynamic slot lowers to the interned value.ErrorConstructor value rather than a
// bare Go identifier, which has no value form.
func TestErrorConstructorBoxesToValue(t *testing.T) {
	src := "let c: any = TypeError; let nm: string = c.name; console.log(nm);"
	out := renderProgram(t, src)
	if !strings.Contains(out, `value.ErrorConstructor("TypeError")`) {
		t.Fatalf("TypeError did not box to the constructor value:\n%s", out)
	}
}

// TestErrorConstructorNameAndIdentityRun builds and runs the assert.throws surface:
// reading .name off a boxed constructor and comparing two constructors for
// identity, so the boxed value answers the name and compares equal to itself.
func TestErrorConstructorNameAndIdentityRun(t *testing.T) {
	skipIfShort(t)
	src := `
let ctor: any = TypeError;
let nm: string = ctor.name;
console.log(nm);
let same: boolean = ctor === TypeError;
console.log(same);
let diff: boolean = ctor === RangeError;
console.log(diff);
`
	got := runProgramGo(t, src)
	want := "TypeError\ntrue\nfalse\n"
	if got != want {
		t.Fatalf("error constructor value run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestErrorConstructorShadowedByClassHandsBack pins the ambient-global gate: a user
// class named like a built-in error constructor is not the built-in, so naming it
// as a value keeps its own lowering rather than boxing to the interned constructor.
func TestErrorConstructorShadowedByClassHandsBack(t *testing.T) {
	src := "function f(): void { let TypeError: number = 5; let c: any = TypeError; console.log(c === c); }"
	out := renderProgram(t, src)
	if strings.Contains(out, "ErrorConstructor") {
		t.Fatalf("a local named TypeError boxed as the built-in constructor:\n%s", out)
	}
	if !strings.Contains(out, "value.Number") {
		t.Fatalf("a local number named TypeError did not box as a number:\n%s", out)
	}
}
