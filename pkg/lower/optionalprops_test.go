package lower

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestOptionalPropertyEmits pins the shape of the lowering: an optional property
// interns a value.Opt field, an object literal in an optional-carrying slot builds
// at the declared shape wrapping a present optional field in value.Some and an
// omitted one in value.None, and a read of the optional member yields the Opt the
// nullish path unwraps with Or.
func TestOptionalPropertyEmits(t *testing.T) {
	const src = `type Point = { x: number; y?: number };
function dist(p: Point): number {
  return p.x + (p.y ?? 0);
}
const a: Point = { x: 3 };
const b: Point = { x: 3, y: 4 };
console.log(dist(a));
console.log(dist(b));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.Opt[float64]",
		"value.Some[float64](4)",
		"value.None[float64]()",
		"p.Y.Or(0)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("optional-property lowering did not print %q:\n%s", want, source)
		}
	}
}

// TestOptionalPropertyHandsBack pins the boundaries this slice keeps: an object
// literal in a T | undefined slot needs the outer boxing, and Object.keys of an
// optional shape needs the key-presence handling its own slice adds, so each hands
// back with a named reason.
func TestOptionalPropertyHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"literalIntoOptionalSlot",
			"type Point = { x: number; y?: number };\nconst a: Point | undefined = { x: 3 };\nconsole.log(a === undefined);\n",
			"T | undefined optional slot",
		},
		{
			"objectKeysOfOptionalShape",
			"type Point = { x: number; y?: number };\nconst a: Point = { x: 3 };\nconsole.log(Object.keys(a).length);\n",
			"optional property",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := compile(t, tc.src)
			r := NewRenderer(prog)
			_, err := r.RenderProgram(entryFile(t, prog))
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
			}
			if !strings.Contains(nyl.Reason, tc.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", nyl.Reason, tc.want)
			}
		})
	}
}

// TestNarrowedOptionalPropertyRuns builds and runs a narrowed read of an optional
// property: inside an x !== undefined guard the read unwraps the value.Opt field with
// .Get(), so a supplied optional reads back its inner value and an omitted one takes the
// else branch, over both a primitive inner and a nested-shape inner.
func TestNarrowedOptionalPropertyRuns(t *testing.T) {
	skipIfShort(t)
	const src = `interface P { name: string; age?: number; }
function tell(p: P): string {
  if (p.age !== undefined) {
    return p.name + ":" + (p.age + 1);
  }
  return p.name + ":none";
}
interface Box { inner?: { val: number }; }
function peek(b: Box): number {
  if (b.inner !== undefined) {
    return b.inner.val;
  }
  return -1;
}
console.log(tell({ name: "amy", age: 30 }));
console.log(tell({ name: "bo" }));
console.log(peek({ inner: { val: 5 } }));
console.log(peek({}));
`
	got := runProgramGo(t, src)
	want := "amy:31\nbo:none\n5\n-1\n"
	if got != want {
		t.Fatalf("narrowed optional-property program printed %q, want %q", got, want)
	}
}

// TestJSONStringifyOptionalShapeEmits pins that JSON.stringify of a shape with an
// optional property now lowers rather than handing back: the serializer's
// reflection walk learned the value.Opt field, so the call emits value.JSONStringify
// over the built struct.
func TestJSONStringifyOptionalShapeEmits(t *testing.T) {
	const src = "type Point = { x: number; y?: number };\nconst a: Point = { x: 3 };\nconsole.log(JSON.stringify(a));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.JSONStringify(a)") {
		t.Errorf("JSON.stringify of an optional shape did not lower to value.JSONStringify(a):\n%s", source)
	}
}

// TestOptionalPropertyRuns builds and runs the optional-property lowering end to
// end: a literal that omits the optional field reads back as undefined and falls
// to the ?? fallback, while one that supplies it keeps the value.
func TestOptionalPropertyRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the test builds and runs generated Go")
	}
	const src = `type Point = { x: number; y?: number };
function dist(p: Point): number {
  return p.x + (p.y ?? 0);
}
const a: Point = { x: 3 };
const b: Point = { x: 3, y: 4 };
console.log(dist(a));
console.log(dist(b));
`
	got := runProgramGo(t, src)
	want := "3\n7\n"
	if got != want {
		t.Fatalf("optional-property program printed %q, want %q", got, want)
	}
}
