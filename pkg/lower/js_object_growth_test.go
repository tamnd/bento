package lower

import (
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// firstObjectLiteralJS returns the first object literal in a JavaScript snippet, the
// mode where the checker folds a later `o.x = 1` back into the literal's type.
func firstObjectLiteralJS(t *testing.T, src string) (*Renderer, frontend.Node) {
	t.Helper()
	prog := compileJS(t, src)
	var lits []frontend.Node
	collectKind(prog, prog.SourceFiles(), frontend.NodeObjectLiteralExpression, &lits)
	if len(lits) == 0 {
		t.Fatal("no object literal in snippet")
	}
	return NewRenderer(prog), lits[0]
}

// TestObjectLiteralGrowsPastItsKeys pins what the predicate claims. A literal that
// builds every property its type names is a fixed shape and belongs on the struct path;
// one that leaves a property to a later assignment is an object that grows, and no
// struct can hold it honestly.
func TestObjectLiteralGrowsPastItsKeys(t *testing.T) {
	// Only an empty literal grows this way: TypeScript folds later assignments into the
	// type of `{}`, and a literal that already names members is left at the shape it
	// was written with, a property added to it being an ordinary 2339.
	grows := []string{
		`const o = {};
o.x = 1;
console.log(o.x);`,
		`const o = {};
o.x = 1;
o.y = "two";
console.log(o.x, o.y);`,
	}
	for _, src := range grows {
		r, lit := firstObjectLiteralJS(t, src)
		if !r.objectLiteralGrowsProperties(lit) {
			t.Errorf("objectLiteralGrowsProperties(%q) = false, want true", src)
		}
	}

	fixed := []string{
		`const o = { a: 1 };
o.a = 2;
console.log(o.a);`,
		`const o = { a: 1, b: 2 };
console.log(o.a, o.b);`,
		`const o = {};
console.log(typeof o);`,
	}
	for _, src := range fixed {
		r, lit := firstObjectLiteralJS(t, src)
		if r.objectLiteralGrowsProperties(lit) {
			t.Errorf("objectLiteralGrowsProperties(%q) = true, want false", src)
		}
	}
}

// TestGrowingObjectBuildsTheRuntimeBag pins the lowering: a growing object is the
// dynamic object the runtime carries, not a Go struct, so the property that arrives
// later has somewhere to land.
func TestGrowingObjectBuildsTheRuntimeBag(t *testing.T) {
	source := renderExpandoJS(t, `const o = {};
o.x = 1;
console.log(o.x);
`)
	if !strings.Contains(source, "value.NewObject()") {
		t.Errorf("a growing object did not build the runtime bag:\n%s", source)
	}
}

// TestGrowingObjectRuns is the end-to-end proof for the idiom every Node module is
// written in: make an object, fill it in afterwards, read it back.
func TestGrowingObjectRuns(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const o = {};
o.x = 1;
o.y = o.x + 1;
console.log(o.x, o.y);
`))
	if got != "1 2\n" {
		t.Errorf("growing object ran wrong\n got: %q\nwant: %q", got, "1 2\n")
	}
}

// TestPropertyReadBeforeItIsAssignedIsUndefined is why the change is a boxing rather
// than a zero-filled struct, and the case worth guarding hardest. The checker types the
// object by its finished shape, so a struct would have the field sitting at 0 from the
// start and the read below would print 0. JavaScript prints undefined, and a compiler
// that answers 0 to a question whose answer is undefined is worse than one that refuses
// the program.
func TestPropertyReadBeforeItIsAssignedIsUndefined(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const o = {};
console.log(o.x);
o.x = 1;
console.log(o.x);
`))
	if got != "undefined\n1\n" {
		t.Errorf("read before assignment\n got: %q\nwant: %q", got, "undefined\n1\n")
	}
}

// TestBracketWriteGrowsTheSameObject pins that the index spelling of the same idiom
// takes the same path, since a Node module reaches for both.
func TestBracketWriteGrowsTheSameObject(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const o = {};
o["a"] = "one";
console.log(o["a"]);
`))
	if got != "one\n" {
		t.Errorf("bracket write\n got: %q\nwant: %q", got, "one\n")
	}
}

// TestAnnotatedOptionalPropertyStaysAStruct pins the other boundary, the TypeScript one.
// A slot that declares a property optional says the property may be absent, and the
// contextual path already fills it with the empty optional. Reading that as growth would
// box every literal written against an interface with an optional member.
func TestAnnotatedOptionalPropertyStaysAStruct(t *testing.T) {
	source := renderProgram(t, `const p: { x: number; y?: number } = { x: 1 };
console.log(p.x);
`)
	if strings.Contains(source, "value.NewObject()") {
		t.Errorf("a literal with an omitted optional property was boxed:\n%s", source)
	}
}

// TestFixedShapeObjectStaysAStruct pins the boundary from the other side. Boxing every
// object literal would be the easy way to make the case above work and would give up
// the static shape the typed path is built on, so an object that never grows must still
// intern a struct.
func TestFixedShapeObjectStaysAStruct(t *testing.T) {
	source := renderExpandoJS(t, `const o = { a: 1, b: 2 };
o.a = 3;
console.log(o.a, o.b);
`)
	if strings.Contains(source, "value.NewObject()") {
		t.Errorf("a fixed-shape object was boxed rather than interned as a struct:\n%s", source)
	}
}
