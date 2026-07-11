package lower

import (
	"strings"
	"testing"
)

// TestBracketMemberReadRoutesToClass pins that a bracket read with a constant
// string key on a class receiver routes to the class member dispatch, the same
// Go the dotted read produces, so a non-identifier or computed member name reads
// through its bracket spelling. The string-named field reads as its struct field
// and the ["g"] get accessor reads as the accessor-method call, not as a bare
// method value the object-shape path would emit.
func TestBracketMemberReadRoutesToClass(t *testing.T) {
	const src = `class C {
  "my field": number = 5;
  get ["g"](): number { return 7; }
}
const c = new C();
console.log(String(c["my field"]));
console.log(String(c["g"]));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "c.MyU20_field") {
		t.Errorf("bracket field read did not route to the struct field:\n%s", source)
	}
	if !strings.Contains(source, "c.G()") {
		t.Errorf("bracket getter read did not route to the accessor call:\n%s", source)
	}
	if strings.Contains(source, "type Obj") {
		t.Errorf("bracket read interned a spurious object shape:\n%s", source)
	}
}

// TestBracketMemberReadRuns pins the runtime behavior end to end: a string-named
// field, a computed-string method call, a static string method call, and a ["g"]
// get accessor read each resolve to the member the class declares.
func TestBracketMemberReadRuns(t *testing.T) {
	const src = `class C {
  "my field": number = 5;
  ["m"](): number { return 2; }
  static "sm"(): number { return 3; }
  get ["g"](): number { return 7; }
}
const c = new C();
console.log(String(c["my field"]));
console.log(String(c["m"]()));
console.log(String(C["sm"]()));
console.log(String(c["g"]));
`
	got := runProgramGo(t, src)
	const want = "5\n2\n3\n7\n"
	if got != want {
		t.Errorf("bracket member program ran wrong\n got: %q\nwant: %q", got, want)
	}
}

// TestComputedLiteralConstName pins that a computed member name [k] whose k is a
// const of a literal string type folds to that string at class-definition time,
// so the declaration and a read agree on the member. The method declared under
// [k] where k is "m" is reachable both as a bracket read c[k]() and as the dotted
// c.m() the folded name spells, and a [k] field reads the same way.
func TestComputedLiteralConstName(t *testing.T) {
	const src = `const mk = "m";
const fk = "f";
class C {
  [mk](): number { return 4; }
  [fk]: number = 6;
}
const c = new C();
console.log(String(c[mk]()));
console.log(String(c.m()));
console.log(String(c[fk]));
`
	got := runProgramGo(t, src)
	const want = "4\n4\n6\n"
	if got != want {
		t.Errorf("computed literal-const name ran wrong\n got: %q\nwant: %q", got, want)
	}
}

// TestSymbolKeyedMemberHandsBack pins the boundary: a symbol-keyed class member,
// whose name the checker types as a unique symbol rather than a constant string,
// stays a later slice and hands back rather than lowering to a wrong name.
func TestSymbolKeyedMemberHandsBack(t *testing.T) {
	const src = `class C {
  static [Symbol.iterator](): number { return 3; }
}
new C();
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "computed member name that is not a constant string") {
		t.Errorf("symbol-keyed member hand-back reason %q does not name the computed-name boundary", reason)
	}
}

// TestBracketAccessorStore pins that a store through a computed or string-named
// set accessor spelled with the bracket, b["val"] = v, routes to the accessor
// method the class declared, b.SetVal(v), the same Set call the dotted store
// takes, and the paired get accessor reads it back.
func TestBracketAccessorStore(t *testing.T) {
	const src = `class Box {
  _v: number = 0;
  get ["val"](): number { return this._v; }
  set ["val"](n: number) { this._v = n; }
}
const b = new Box();
b["val"] = 9;
console.log(String(b["val"]));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "b.SetVal(9)") {
		t.Errorf("bracket accessor store did not route to the setter method:\n%s", source)
	}
	got := runProgramGo(t, src)
	if got != "9\n" {
		t.Errorf("bracket accessor store ran wrong\n got: %q\nwant: %q", got, "9\n")
	}
}

// TestBracketMethodCallThroughThis pins that a bracket method call through this
// inside a class body, this["m x"](), dispatches to the Go method the class
// declared rather than handing back the way a bare method read does.
func TestBracketMethodCallThroughThis(t *testing.T) {
	const src = `class C {
  "m x"(): number { return 8; }
  run(): number { return this["m x"](); }
}
console.log(String(new C().run()));
`
	got := runProgramGo(t, src)
	if got != "8\n" {
		t.Errorf("bracket method call through this ran wrong\n got: %q\nwant: %q", got, "8\n")
	}
}
