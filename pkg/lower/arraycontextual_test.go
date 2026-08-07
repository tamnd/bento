package lower

import (
	"strings"
	"testing"
)

// TestNonEmptyArrayLiteralReturnsBoxed pins that a concretely-typed array literal
// returned as any[] re-emits at value.Value elements. The checker types [1] as
// float64[], which the *value.Array[value.Value] return slot rejects, so the literal
// boxes each element and rebuilds as value.NewArray[value.Value].
func TestNonEmptyArrayLiteralReturnsBoxed(t *testing.T) {
	const src = `function foo(): any[] { return [1]; }
console.log(foo().length);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewArray[value.Value](value.Number(1))") {
		t.Errorf("array literal returned as any[] did not box its element:\n%s", source)
	}
}

// TestNonEmptyArrayLiteralArgBoxesRuns builds and runs a mixed literal flowing into an
// any[] parameter through a call: each element boxes to value.Value, so the argument
// fits the boxed header and the program reads the elements back.
func TestNonEmptyArrayLiteralArgBoxesRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function func1(stuff: any[]) { return stuff; }
function func2(a: string, b: number, c: number) {
  return func1([a, b, c]);
}
const r = func2("3", 1, 2);
console.log(r.length, r[0], r[1]);
`
	if got, want := runProgramGo(t, src), "3 3 1\n"; got != want {
		t.Fatalf("any[] arg boxing run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestUnionElementArrayLiteralRebuilds pins the union end of the contextual rebuild.
// The checker types ["pwd", []] by its own contents, an array of string | never[],
// which the (string | string[])[] slot rejects at go build because the two unions
// intern different Go structs. The literal re-emits at the slot's element instead, so
// both elements reach it through that union's own arm constructors.
func TestUnionElementArrayLiteralRebuilds(t *testing.T) {
	const src = `function cmd(): (string | string[])[] { return ["pwd", ["a"]]; }
console.log(cmd().length);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewArray[StrArrOrStr](StrArrOrStrOfStr(") {
		t.Errorf("the literal did not rebuild at the slot's union element:\n%s", source)
	}
	if strings.Contains(source, "value.NewArray[value.Value]") {
		t.Errorf("an element widened to a box instead of taking its arm:\n%s", source)
	}
}

// TestEmptyArrayElementTakesItsUnionArm pins the bare [] inside such a literal. An
// empty literal carries no contextual type, so the checker types it never[], whose
// structural key names no arm; the union's one array member says which arm it meant
// and the literal is re-spelled at that member's element type.
func TestEmptyArrayElementTakesItsUnionArm(t *testing.T) {
	const src = `function cmd(): (string | string[])[] { return ["pwd", []]; }
console.log(cmd().length);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "StrArrOrStrOfStrArr(value.NewArray[value.BStr]())") {
		t.Errorf("the bare [] did not take the array arm at its element type:\n%s", source)
	}
}

// TestTwoArrayMembersLeaveTheBareArrayRefused pins the bar. Two array members give an
// empty literal no one answer, and the literal carries no contextual type of its own,
// so the union keeps its hand back rather than pick an arm at random.
func TestTwoArrayMembersLeaveTheBareArrayRefused(t *testing.T) {
	const src = `function cmd(): (string[] | number[])[] { return [[]]; }
console.log(cmd().length);
`
	if reason := renderProgramHandBack(t, src); reason == "" {
		t.Fatalf("want a hand back for two array members, got none")
	}
}
