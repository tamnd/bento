package lower

import (
	"errors"
	"strings"
	"testing"
)

// A destructured parameter has no Go equivalent, so the whole object or array
// arrives in one synthesized field and the names the pattern bound are read out of
// it at the top of the body. The reads are the same struct-field selectors and
// indexed reads a `const {a} = o` or `const [x] = xs` statement lowers to.

// TestDestructuredParamLowersToEntryBindings proves an object-pattern parameter
// and an array-pattern parameter each bind their names from the held field at body
// entry rather than leaving the names undefined.
func TestDestructuredParamLowersToEntryBindings(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		wants []string
	}{
		{
			"object",
			"function area({w, h}: {w: number, h: number}): number { return w * h; }\nconsole.log(area({w: 3, h: 4}));\n",
			[]string{"w := __0.W", "h := __0.H"},
		},
		{
			"array",
			"function diff([x, y]: number[]): number { return x - y; }\nconsole.log(diff([9, 4]));\n",
			[]string{"x := __0.AtI(0)", "y := __0.AtI(1)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			for _, want := range tc.wants {
				if !strings.Contains(source, want) {
					t.Errorf("destructured parameter did not print %q:\n%s", want, source)
				}
			}
		})
	}
}

// TestDestructuredParamRuns builds and runs object, array, and mixed patterns so
// each bound name is proven to carry the right field or element against the
// JavaScript result rather than just the emitted shape.
func TestDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function area({w, h}: {w: number, h: number}): number {
  return w * h;
}
function diff([x, y]: number[]): number {
  return x - y;
}
function label({name, id}: {name: string, id: number}): string {
  return name + id;
}
function shift(base: number, {by}: {by: number}): number {
  return base + by;
}
console.log(area({w: 3, h: 4}));
console.log(diff([9, 4]));
console.log(label({name: "n", id: 7}));
console.log(shift(10, {by: 5}));
`
	if got, want := runProgramGo(t, src), "12\n5\nn7\n15\n"; got != want {
		t.Fatalf("destructured parameter program printed %q, want %q", got, want)
	}
}

// TestDestructuredParamRenameHandsBack proves the shapes the shorthand binding does
// not cover still hand back: a rename in an object pattern and a nested array
// pattern each name their own later slice rather than emit an unbound read.
func TestDestructuredParamRenameHandsBack(t *testing.T) {
	for _, src := range []string{
		"function f({a: x}: {a: number}): number { return x; }\nf({a: 1});\n",
		"function f([[a]]: number[][]): number { return a; }\nf([[1]]);\n",
	} {
		prog := compile(t, src)
		r := NewRenderer(prog)
		_, err := r.RenderProgram(entryFile(t, prog))
		var nyl *NotYetLowerable
		if !errors.As(err, &nyl) {
			t.Fatalf("RenderProgram(%q) err = %v, want a *NotYetLowerable", src, err)
		}
	}
}
