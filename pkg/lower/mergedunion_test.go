package lower

import (
	"strings"
	"testing"
)

// A union of object shapes that all declare the same keys has no discriminant and needs
// none: the checker already answers a read of any key with the union of what the members
// hold there, so the union lowers to one struct whose fields are those unions. These
// tests pin the construction sites that have to build at that merged struct rather than
// at the member shape a literal looks like on its own, and the limits that stay refused.

const likeKeyed = "type U = { a: number, b: string } | { a: string, b: number };\n"

// TestALikeKeyedObjectUnionLowersToOneStruct is the shape itself. Both keys carry the
// merge of what the two members hold, so neither field is a member's own Go type.
func TestALikeKeyedObjectUnionLowersToOneStruct(t *testing.T) {
	got := renderProgram(t, likeKeyed+`const u: U = { a: 1, b: "y" };
console.log(u.a, u.b);`)
	if !strings.Contains(got, "A NumOrStr") || !strings.Contains(got, "B NumOrStr") {
		t.Errorf("the merged struct did not carry the union at both keys:\n%s", got)
	}
}

// TestATernaryOverLikeKeyedObjectsBuildsBothBranchesAtTheMerge is the shape node's own
// test harness opens with, `typeof ms === "bigint" ? { two: 2n } : { two: 2 }`. The IIFE
// has one Go result type, so a branch built at its own member would not fit it.
func TestATernaryOverLikeKeyedObjectsBuildsBothBranchesAtTheMerge(t *testing.T) {
	got := renderProgram(t, `const ms: number | bigint = 2;
const m = typeof ms === "bigint" ? { two: 2n, four: 4n } : { two: 2, four: 4 };
console.log(typeof m.two, typeof m.four);`)
	if strings.Count(got, "type ObjFourTwo struct") != 1 {
		t.Errorf("the ternary did not settle on one struct:\n%s", got)
	}
	if strings.Contains(got, "ObjFourTwo_2{") || strings.Contains(got, "ObjFourTwo_3{") {
		t.Errorf("a branch built at its own member shape:\n%s", got)
	}
}

// TestEveryConstructionSiteBuildsAtTheMerge walks the slots a value can enter through.
// Each one is a place the literal's own fresh type interns a different struct than the
// slot, so each one needs the contextual build.
func TestEveryConstructionSiteBuildsAtTheMerge(t *testing.T) {
	cases := []struct{ name, src string }{
		{"a binding", `const u: U = { a: 1, b: "y" };
console.log(u.a);`},
		{"a return", `function f(x: boolean): U { if (x) { return { a: 1, b: "y" }; } return { a: "z", b: 2 }; }
console.log(f(true).a);`},
		{"a returned ternary", `function f(x: boolean): U { return x ? { a: 1, b: "y" } : { a: "z", b: 2 }; }
console.log(f(true).a);`},
		{"an argument", `function g(u: U): string { return typeof u.a; }
console.log(g({ a: 1, b: "y" }));`},
		{"an assignment", `let u: U = { a: 1, b: "y" };
u = { a: "z", b: 2 };
console.log(u.a);`},
		{"an array element", `const xs: U[] = [{ a: 1, b: "y" }, { a: "z", b: 2 }];
console.log(xs[0].a);`},
		{"a nested field", `type W = { u: U, n: number };
const w: W = { u: { a: 1, b: "y" }, n: 3 };
console.log(w.u.a, w.n);`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderProgram(t, likeKeyed+tc.src)
			// The merged struct is the only ObjAB the program should build. A member shape
			// interns as ObjAB_2 or ObjAB_3, and seeing one of those built is the bug this
			// slice fixes: it is a Go value of the wrong struct going into the slot.
			if strings.Contains(got, "ObjAB_2{") || strings.Contains(got, "ObjAB_3{") {
				t.Errorf("built at a member shape rather than the merge:\n%s", got)
			}
			if !strings.Contains(got, "NumOrStrOfNum(") {
				t.Errorf("no member named its arm, so nothing built at the merge:\n%s", got)
			}
		})
	}
}

// TestANarrowedReadOffTheMergeSelectsTheArm is the read side. Narrowing the receiver to
// one member narrows every read off it, but the Go value in the slot is the merged
// struct for the whole of its life, so the read has to select the arm the narrowing
// names rather than hand the whole sum to a sink expecting a float64.
func TestANarrowedReadOffTheMergeSelectsTheArm(t *testing.T) {
	got := renderProgram(t, likeKeyed+`const u: U = { a: 1, b: "y" };
console.log(u.a, u.b);`)
	if !strings.Contains(got, ".A.num") || !strings.Contains(got, ".B.str") {
		t.Errorf("a narrowed read did not select its arm:\n%s", got)
	}
}

// TestADiscriminatedUnionKeepsItsTag is the line the merge stops at. Narrowing on a
// discriminant is real information a merge would throw away, so a union that has one
// stays with the tagged sum whatever its key sets look like.
func TestADiscriminatedUnionKeepsItsTag(t *testing.T) {
	got := renderProgram(t, `type S = { kind: "c", v: number } | { kind: "s", v: string };
const s: S = { kind: "c", v: 1 };
console.log(s.kind);`)
	if strings.Contains(got, "V NumOrStr") {
		t.Errorf("a discriminated union was merged:\n%s", got)
	}
}

// TestUnionsThatAreNotLikeKeyedStayRefused pins the two shapes the merge declines.
// A differing key set would grow a field on a value that does not have one, and the
// `in` operator, which is how the language tells those apart, would then answer for a
// key the shape carries but the value never set.
func TestUnionsThatAreNotLikeKeyedStayRefused(t *testing.T) {
	// The union has to be written where it is rendered rather than only annotated on a
	// binding: a binding the checker narrows to one member never asks for the union's
	// own Go type, so the annotation alone proves nothing either way.
	cases := []struct{ name, src string }{
		{"a differing key set", `type D = { a: number } | { a: number, b: string };
function f(x: boolean): D { if (x) { return { a: 1 }; } return { a: 1, b: "y" }; }
console.log(f(true).a);`},
		{"an object beside a primitive", `type P = { a: number } | number;
function f(x: boolean): P { if (x) { return { a: 1 }; } return 2; }
console.log(typeof f(true));`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := renderProgramHandBack(t, tc.src)
			if reason == "" {
				t.Fatalf("want a hand back, got none")
			}
		})
	}
}

// TestASpreadOfTheMergeHandsBack keeps the copy honest. A field read off the merge is
// the tagged sum, not the arm, so copying it into a literal the checker typed at one
// member would drop the sum into a slot of that arm's own Go type.
func TestASpreadOfTheMergeHandsBack(t *testing.T) {
	reason := renderProgramHandBack(t, likeKeyed+`const u: U = { a: 1, b: "y" };
const v = { ...u };
console.log(typeof v.a);`)
	if !strings.Contains(reason, "like-keyed object union") {
		t.Errorf("hand back said %q, want it to name the union", reason)
	}
}
