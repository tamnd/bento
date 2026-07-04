package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestArrayStaticEmits pins the shape: Array.of builds the same value.NewArray
// an array literal does, instantiated at the checker's element type, so the
// arguments become the elements one to one.
func TestArrayStaticEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"of",
			"export function trio(): number[] { return Array.of(1, 2, 3); }\n",
			"value.NewArray[float64](1, 2, 3)",
		},
		{
			"ofStrings",
			"export function names(): string[] { return Array.of(\"a\", \"b\"); }\n",
			"value.NewArray[value.BStr](value.FromGoString(\"a\"), value.FromGoString(\"b\"))",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("Array static did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestArrayStaticHandsBack pins the boundary: Array.from, whose general form
// walks an iterable or array-like and takes an optional map function, is a later
// slice, so it hands back with a named reason rather than falling through to the
// receiver-typed method dispatch.
func TestArrayStaticHandsBack(t *testing.T) {
	const src = "export function copy(a: number[]): number[] { return Array.from(a); }\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "Array.from is a later slice") {
		t.Errorf("hand-back reason = %q, want it to mention Array.from", nyl.Reason)
	}
}
