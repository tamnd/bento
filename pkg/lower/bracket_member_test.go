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
